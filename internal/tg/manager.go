package tg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"golang.org/x/net/proxy"

	"tdload/internal/config"
	"tdload/internal/db"
)

const defaultAccountID int64 = 1

// Telegram Desktop 公开凭证（与 iyear/tdl、config.DesktopApp* 相同）。
const (
	DesktopAppID   = config.DesktopAppID
	DesktopAppHash = config.DesktopAppHash
)

type Manager struct {
	Cfg *config.Config
	DB  *db.DB

	mu      sync.Mutex
	pending map[string]*pendingCode
	// clientMu 串行化所有 MTProto 连接，避免 Status / 下载 / 登录抢同一 session 文件导致失效
	clientMu sync.Mutex
}

type pendingCode struct {
	Phone     string
	CodeHash  string
	CreatedAt time.Time
}

type UserInfo struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	Phone     string `json:"phone"`
}

func NewManager(cfg *config.Config, database *db.DB) *Manager {
	return &Manager{
		Cfg:     cfg,
		DB:      database,
		pending: map[string]*pendingCode{},
	}
}

func (m *Manager) Configured() bool {
	return m.Cfg.AppID > 0 && strings.TrimSpace(m.Cfg.AppHash) != ""
}

func (m *Manager) UsingDesktopPreset() bool {
	return m.Cfg.AppID == DesktopAppID && m.Cfg.AppHash == DesktopAppHash
}

// UseDesktopPreset 写入 Telegram Desktop 内置 api_id/hash，跳过 my.telegram.org。
func (m *Manager) UseDesktopPreset() error {
	m.Cfg.AppID = DesktopAppID
	m.Cfg.AppHash = DesktopAppHash
	return m.Cfg.Save()
}

func (m *Manager) sessionPath() string {
	return filepath.Join(m.Cfg.SessionDir, fmt.Sprintf("account_%d.json", defaultAccountID))
}

func (m *Manager) newClient() (*telegram.Client, error) {
	if !m.Configured() {
		return nil, fmt.Errorf("请先配置 API 凭证，或点击「使用 Desktop 内置凭证」")
	}
	if err := os.MkdirAll(m.Cfg.SessionDir, 0o755); err != nil {
		return nil, err
	}

	device := telegram.DeviceConfig{
		DeviceModel:    "TDLoad",
		SystemVersion:  "1.0",
		AppVersion:     "0.1.0",
		LangCode:       "zh",
		SystemLangCode: "zh-CN",
	}
	if m.UsingDesktopPreset() {
		// 与官方 Desktop 客户端标识更接近，降低异常风控概率
		device.DeviceModel = "Desktop"
		device.SystemVersion = "Windows 10"
		device.AppVersion = "4.0.2 x64"
		device.LangCode = "en"
		device.SystemLangCode = "en"
	}

	opts := telegram.Options{
		SessionStorage: &session.FileStorage{Path: m.sessionPath()},
		Device:         device,
	}

	if p := strings.TrimSpace(m.Cfg.Proxy); p != "" {
		d, err := proxyFromURL(p)
		if err != nil {
			return nil, errors.Wrap(err, "proxy")
		}
		opts.Resolver = dcs.Plain(dcs.PlainOptions{Dial: d.DialContext})
	}

	return telegram.NewClient(m.Cfg.AppID, m.Cfg.AppHash, opts), nil
}

func proxyFromURL(raw string) (proxy.ContextDialer, error) {
	u, err := parseProxyURL(raw)
	if err != nil {
		return nil, err
	}
	var auth *proxy.Auth
	if u.User != nil {
		pass, _ := u.User.Password()
		auth = &proxy.Auth{User: u.User.Username(), Password: pass}
	}
	var dialer proxy.Dialer
	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h":
		dialer, err = proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
	case "http", "https":
		// x/net/proxy 无内置 HTTP；回退为直连并提示改用 socks5
		return nil, fmt.Errorf("请使用 socks5:// 代理（当前不支持 http 代理）")
	default:
		return nil, fmt.Errorf("不支持的代理协议: %s", u.Scheme)
	}
	if err != nil {
		return nil, err
	}
	cd, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("proxy dialer 不支持 Context")
	}
	return cd, nil
}

type urlParts struct {
	Scheme string
	Host   string
	User   *urlUser
}

type urlUser struct {
	username string
	password string
	hasPass  bool
}

func (u *urlUser) Username() string { return u.username }
func (u *urlUser) Password() (string, bool) {
	return u.password, u.hasPass
}

func parseProxyURL(raw string) (*urlParts, error) {
	// 简易解析 socks5://user:pass@host:port
	raw = strings.TrimSpace(raw)
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return nil, fmt.Errorf("代理 URL 格式错误")
	}
	out := &urlParts{Scheme: scheme}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		userinfo, host := rest[:at], rest[at+1:]
		out.Host = host
		user, pass, has := strings.Cut(userinfo, ":")
		out.User = &urlUser{username: user, password: pass, hasPass: has}
	} else {
		out.Host = rest
	}
	if out.Host == "" {
		return nil, fmt.Errorf("代理缺少 host")
	}
	return out, nil
}

func (m *Manager) Status(ctx context.Context) (map[string]any, error) {
	out := map[string]any{
		"configured":         m.Configured(),
		"loggedIn":           false,
		"status":             "none",
		"message":            "",
		"user":               nil,
		"usingDesktopPreset": m.UsingDesktopPreset(),
	}
	if !m.Configured() {
		out["message"] = "尚未配置 app_id / app_hash"
		return out, nil
	}

	var prevActive *db.TGAccount
	acc, err := m.DB.GetTGAccount(ctx, defaultAccountID)
	if err == nil && acc != nil {
		out["status"] = acc.Status
		if acc.Status == "active" {
			prevActive = acc
			out["loggedIn"] = true
			out["user"] = UserInfo{
				ID:       acc.UserID,
				Username: acc.Label,
				Phone:    acc.Phone,
			}
			out["message"] = "已登录（本地记录）"
		}
	}

	if _, err := os.Stat(m.sessionPath()); os.IsNotExist(err) {
		out["loggedIn"] = false
		out["status"] = "none"
		out["message"] = "未登录"
		out["user"] = nil
		return out, nil
	}

	// 下载占用客户端时不要阻塞，直接返回本地状态
	if !m.clientMu.TryLock() {
		if prevActive != nil {
			out["message"] = "客户端忙碌（下载中），显示本地登录状态"
			return out, nil
		}
		out["message"] = "客户端忙碌，请稍后刷新"
		return out, nil
	}
	defer m.clientMu.Unlock()

	client, err := m.newClient()
	if err != nil {
		out["message"] = err.Error()
		return out, nil
	}

	// 在线探测必须短超时，否则会长时间占锁把下载堵死
	probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer probeCancel()

	runErr := client.Run(probeCtx, func(ctx context.Context) error {
		st, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !st.Authorized {
			out["loggedIn"] = false
			out["status"] = "expired"
			out["message"] = "会话无效，请重新登录"
			out["user"] = nil
			_ = m.DB.UpsertTGAccount(ctx, &db.TGAccount{
				ID:     defaultAccountID,
				Status: "expired",
			})
			return nil
		}
		self, err := client.Self(ctx)
		if err != nil {
			return err
		}
		info := userFromSelf(self)
		out["loggedIn"] = true
		out["status"] = "active"
		out["user"] = info
		out["message"] = fmt.Sprintf("已登录：%s", displayName(info))
		_ = m.DB.UpsertTGAccount(ctx, &db.TGAccount{
			ID:          defaultAccountID,
			Label:       displayName(info),
			Phone:       info.Phone,
			UserID:      info.ID,
			SessionFile: m.sessionPath(),
			Status:      "active",
		})
		return nil
	})
	if runErr != nil {
		// 网络抖动等：不要误判为过期，保留本地 active
		if prevActive != nil {
			out["loggedIn"] = true
			out["status"] = "active"
			out["message"] = "探测暂时失败，沿用本地登录状态：" + runErr.Error()
			return out, nil
		}
		if out["message"] == "" || out["message"] == "已登录（本地记录）" {
			out["message"] = "未登录或探测失败：" + runErr.Error()
			out["status"] = "none"
			out["loggedIn"] = false
		}
	}
	return out, nil
}

func (m *Manager) SendCode(ctx context.Context, phone string) (loginID string, err error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return "", fmt.Errorf("请输入手机号（含国际区号，如 +86...）")
	}

	if err := m.lockClient(ctx); err != nil {
		return "", err
	}
	defer m.clientMu.Unlock()

	client, err := m.newClient()
	if err != nil {
		return "", err
	}

	var codeHash string
	err = client.Run(ctx, func(ctx context.Context) error {
		sent, err := client.Auth().SendCode(ctx, phone, auth.SendCodeOptions{})
		if err != nil {
			return err
		}
		switch s := sent.(type) {
		case *tg.AuthSentCode:
			codeHash = s.PhoneCodeHash
			return nil
		case *tg.AuthSentCodeSuccess:
			return fmt.Errorf("该号码已自动授权，请刷新状态")
		default:
			return fmt.Errorf("未知的 SendCode 响应: %T", sent)
		}
	})
	if err != nil {
		return "", friendlyAuthErr(err)
	}

	id, err := newID()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.pending[id] = &pendingCode{Phone: phone, CodeHash: codeHash, CreatedAt: time.Now()}
	m.mu.Unlock()
	return id, nil
}

type SignInResult struct {
	NeedPassword bool      `json:"needPassword"`
	User         *UserInfo `json:"user,omitempty"`
}

func (m *Manager) SignIn(ctx context.Context, loginID, code, password string) (*SignInResult, error) {
	m.mu.Lock()
	p := m.pending[loginID]
	m.mu.Unlock()
	if p == nil || time.Since(p.CreatedAt) > 15*time.Minute {
		return nil, fmt.Errorf("登录已过期，请重新发送验证码")
	}
	code = strings.TrimSpace(code)
	password = strings.TrimSpace(password)
	if code == "" && password == "" {
		return nil, fmt.Errorf("请输入验证码")
	}

	if err := m.lockClient(ctx); err != nil {
		return nil, err
	}
	defer m.clientMu.Unlock()

	client, err := m.newClient()
	if err != nil {
		return nil, err
	}

	result := &SignInResult{}
	err = client.Run(ctx, func(ctx context.Context) error {
		if password != "" && code == "" {
			// 仅 2FA
			if _, err := client.Auth().Password(ctx, password); err != nil {
				return err
			}
		} else {
			_, signErr := client.Auth().SignIn(ctx, p.Phone, code, p.CodeHash)
			if errors.Is(signErr, auth.ErrPasswordAuthNeeded) || tgerr.Is(signErr, "SESSION_PASSWORD_NEEDED") {
				if password == "" {
					result.NeedPassword = true
					return nil
				}
				if _, err := client.Auth().Password(ctx, password); err != nil {
					return err
				}
			} else if signErr != nil {
				return signErr
			}
		}

		self, err := client.Self(ctx)
		if err != nil {
			return err
		}
		info := userFromSelf(self)
		result.User = &info
		return m.DB.UpsertTGAccount(ctx, &db.TGAccount{
			ID:          defaultAccountID,
			Label:       displayName(info),
			Phone:       p.Phone,
			UserID:      info.ID,
			SessionFile: m.sessionPath(),
			Status:      "active",
		})
	})
	if err != nil {
		return nil, friendlyAuthErr(err)
	}
	if result.User != nil {
		m.mu.Lock()
		delete(m.pending, loginID)
		m.mu.Unlock()
	}
	return result, nil
}

func (m *Manager) Logout(ctx context.Context) error {
	if err := m.lockClient(ctx); err != nil {
		return err
	}
	defer m.clientMu.Unlock()

	client, err := m.newClient()
	if err != nil {
		// 仍清理本地
		_ = os.Remove(m.sessionPath())
		_ = m.DB.UpsertTGAccount(ctx, &db.TGAccount{ID: defaultAccountID, Status: "none"})
		return nil
	}
	_ = client.Run(ctx, func(ctx context.Context) error {
		_, _ = client.API().AuthLogOut(ctx)
		return nil
	})
	_ = os.Remove(m.sessionPath())
	return m.DB.UpsertTGAccount(ctx, &db.TGAccount{
		ID:     defaultAccountID,
		Status: "none",
	})
}

func userFromSelf(u *tg.User) UserInfo {
	return UserInfo{
		ID:        u.ID,
		Username:  u.Username,
		FirstName: u.FirstName,
		Phone:     u.Phone,
	}
}

func displayName(u UserInfo) string {
	if u.Username != "" {
		return "@" + u.Username
	}
	if u.FirstName != "" {
		return u.FirstName
	}
	return fmt.Sprintf("id:%d", u.ID)
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func friendlyAuthErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case tgerr.Is(err, "PHONE_NUMBER_INVALID"):
		return fmt.Errorf("手机号无效")
	case tgerr.Is(err, "PHONE_CODE_INVALID"):
		return fmt.Errorf("验证码错误")
	case tgerr.Is(err, "PHONE_CODE_EXPIRED"):
		return fmt.Errorf("验证码已过期，请重新发送")
	case tgerr.Is(err, "FLOOD_WAIT"):
		return fmt.Errorf("请求过于频繁，请稍后再试（%s）", msg)
	case strings.Contains(msg, "PASSWORD"):
		return fmt.Errorf("两步验证密码错误")
	default:
		return err
	}
}

func isChatAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case tgerr.Is(err, "CHANNEL_PRIVATE"),
		tgerr.Is(err, "CHAT_PRIVATE"),
		tgerr.Is(err, "CHANNEL_INVALID"),
		tgerr.Is(err, "CHAT_ID_INVALID"),
		tgerr.Is(err, "PEER_ID_INVALID"),
		tgerr.Is(err, "USER_BANNED_IN_CHANNEL"),
		tgerr.Is(err, "CHAT_ADMIN_REQUIRED"),
		tgerr.Is(err, "CHAT_GUEST_SEND_FORBIDDEN"),
		tgerr.Is(err, "CHAT_WRITE_FORBIDDEN"),
		tgerr.Is(err, "CHANNEL_PUBLIC_GROUP_NA"):
		return true
	}
	up := strings.ToUpper(err.Error())
	for _, key := range []string{
		"CHANNEL_PRIVATE", "CHAT_PRIVATE", "CHANNEL_INVALID",
		"PEER_ID_INVALID", "CHAT_ID_INVALID", "USER_BANNED_IN_CHANNEL",
	} {
		if strings.Contains(up, key) {
			return true
		}
	}
	return false
}

// friendlyChatAccessErr 将无权访问 / 非公开 / 无效频道等错误转为可读中文。
func friendlyChatAccessErr(err error) error {
	if err == nil {
		return nil
	}
	if d, ok := tgerr.AsFloodWait(err); ok {
		sec := int(d.Seconds())
		if sec < 1 {
			sec = 1
		}
		return fmt.Errorf("请求过于频繁，请 %d 秒后再试", sec)
	}
	switch {
	case tgerr.Is(err, "CHANNEL_PRIVATE"), tgerr.Is(err, "CHAT_PRIVATE"),
		tgerr.Is(err, "CHANNEL_FORBIDDEN"), tgerr.Is(err, "CHAT_FORBIDDEN"),
		strings.Contains(strings.ToUpper(err.Error()), "CHANNEL_PRIVATE"),
		strings.Contains(strings.ToUpper(err.Error()), "CHAT_PRIVATE"),
		strings.Contains(strings.ToUpper(err.Error()), "CHANNEL_FORBIDDEN"),
		strings.Contains(strings.ToUpper(err.Error()), "CHAT_FORBIDDEN"):
		return fmt.Errorf("无法访问该频道/群组：未加入或为非公开频道，请先加入后再试")
	case tgerr.Is(err, "USER_BANNED_IN_CHANNEL"):
		return fmt.Errorf("账号已被该频道/群组封禁，无法访问")
	case tgerr.Is(err, "USERNAME_INVALID"), tgerr.Is(err, "USERNAME_NOT_OCCUPIED"):
		return fmt.Errorf("用户名不存在或无效")
	case tgerr.Is(err, "CHANNEL_INVALID"), tgerr.Is(err, "CHAT_ID_INVALID"), tgerr.Is(err, "PEER_ID_INVALID"),
		tgerr.Is(err, "CHANNEL_PUBLIC_GROUP_NA"):
		return fmt.Errorf("频道/群组无效或无权访问（非公开频道需先加入；公开频道可填 @用户名）")
	case tgerr.Is(err, "CHAT_ADMIN_REQUIRED"), tgerr.Is(err, "CHAT_WRITE_FORBIDDEN"),
		tgerr.Is(err, "CHAT_GUEST_SEND_FORBIDDEN"):
		return fmt.Errorf("没有访问该频道/群组的权限")
	case isChatAccessDenied(err):
		return fmt.Errorf("无法访问该频道/群组：未加入、非公开或没有权限")
	default:
		return err
	}
}
