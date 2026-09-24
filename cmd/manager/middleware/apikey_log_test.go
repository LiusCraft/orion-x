package middleware

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/logging"
)

// TestAPIKeyNeverAppearsInLogs 是 §7 V2 / R3 的日志侧断言：失败路径只允许打印公开段
// （lookup），完整串一旦出现在日志里就是事故——日志会被采集、会被贴进 issue。
//
// 中间件里唯一一处与 key 有关的日志在 authenticateAPIKey 的失败分支，所以这里制造
// 一次"已撤销"的请求，再检查落盘的日志。
func TestAPIKeyNeverAppearsInLogs(t *testing.T) {
	svc, st, plaintext, _ := keyFixture(t, []string{apikey.ScopeAgentRead}, apikey.Config{})
	r := identityRoute(svc, agentReadTable)

	// zap 在 Init 时就把 os.Stderr 锁进它的 core，所以必须先把 stderr 换掉再 Init。
	original := os.Stderr
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = writePipe
	t.Cleanup(func() {
		// 先换回真 stderr 再重建 logger，免得后面的用例往已关闭的管道里写。
		os.Stderr = original
		if err := logging.Init(logging.Config{Level: "warn", Format: "console"}); err != nil {
			t.Errorf("restore logger: %v", err)
		}
		_ = readPipe.Close()
		_ = writePipe.Close()
	})
	if err := logging.Init(logging.Config{Level: "debug", Format: "console"}); err != nil {
		t.Fatalf("logging.Init: %v", err)
	}

	revokedAt := time.Now()
	st.row.RevokedAt = &revokedAt

	w := do(r, plaintext)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key = %d, want 401 (body %s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), plaintext) {
		t.Fatal("response body leaks the plaintext key")
	}

	logging.Sync()
	if err := writePipe.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	captured, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatalf("read log output: %v", err)
	}
	log := string(captured)

	// 先确认真的抓到了那行日志，否则"没泄漏"只是因为什么都没记。
	if !strings.Contains(log, "apikey: authenticate failed") {
		t.Fatalf("expected the failure to be logged, got: %q", log)
	}
	if strings.Contains(log, plaintext) {
		t.Fatalf("log output contains the full key: %q", log)
	}
	lookup, _, ok := apikey.Parse(plaintext)
	if !ok {
		t.Fatal("fixture is not parseable")
	}
	if !strings.Contains(log, lookup) {
		t.Fatalf("log output should carry the public segment %q for troubleshooting, got: %q", lookup, log)
	}
}
