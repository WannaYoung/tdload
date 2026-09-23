package tg

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gotd/td/tgerr"
)

func TestFriendlyChatAccessErr(t *testing.T) {
	cases := []struct {
		in   error
		want string
	}{
		{tgerr.New(400, "CHANNEL_PRIVATE"), "无法访问该频道/群组：未加入或为非公开频道，请先加入后再试"},
		{tgerr.New(400, "CHANNEL_FORBIDDEN"), "无法访问该频道/群组：未加入或为非公开频道，请先加入后再试"},
		{tgerr.New(400, "USERNAME_NOT_OCCUPIED"), "用户名不存在或无效"},
		{tgerr.New(400, "PEER_ID_INVALID"), "频道/群组无效或无权访问（非公开频道需先加入；公开频道可填 @用户名）"},
		{tgerr.New(400, "USER_BANNED_IN_CHANNEL"), "账号已被该频道/群组封禁，无法访问"},
		{fmt.Errorf("找不到频道 123"), "找不到频道 123"},
		{tgerr.New(420, "FLOOD_WAIT"), "请求过于频繁，请 1 秒后再试"},
	}
	for _, c := range cases {
		got := friendlyChatAccessErr(c.in)
		if got == nil || got.Error() != c.want {
			t.Fatalf("in=%v got=%v want=%q", c.in, got, c.want)
		}
	}
	if friendlyChatAccessErr(nil) != nil {
		t.Fatal("nil")
	}
	if !isChatAccessDenied(tgerr.New(400, "CHANNEL_PRIVATE")) {
		t.Fatal("expected access denied")
	}
	if isChatAccessDenied(errors.New("other")) {
		t.Fatal("unexpected")
	}
}
