package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"tdload/internal/tg"
)

func (s *Server) handleTGStatus(w http.ResponseWriter, r *http.Request) {
	if s.TG == nil {
		writeErr(w, http.StatusServiceUnavailable, "Telegram 模块未初始化")
		return
	}
	st, err := s.TG.Status(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, st)
}

func (s *Server) handleTGSendCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "无效请求体")
		return
	}
	loginID, err := s.TG.SendCode(r.Context(), body.Phone)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w, map[string]any{
		"loginId": loginID,
		"phone":   strings.TrimSpace(body.Phone),
		"message": "验证码已发送，请查收 Telegram / 短信",
	})
}

func (s *Server) handleTGSignIn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LoginID  string `json:"loginId"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "无效请求体")
		return
	}
	res, err := s.TG.SignIn(r.Context(), body.LoginID, body.Code, body.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w, res)
}

func (s *Server) handleTGLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.TG.Logout(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleTGUseDesktop(w http.ResponseWriter, r *http.Request) {
	if s.TG == nil {
		writeErr(w, http.StatusServiceUnavailable, "Telegram 模块未初始化")
		return
	}
	if err := s.TG.UseDesktopPreset(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{
		"configured":         true,
		"usingDesktopPreset": true,
		"appId":              tg.DesktopAppID,
		"message":            "已写入 Desktop 公开凭证，可直接发验证码登录",
	})
}
