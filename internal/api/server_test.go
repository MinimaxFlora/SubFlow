package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/subflow-org/subflow/internal/fetch"
	"github.com/subflow-org/subflow/internal/proxy"
	"github.com/subflow-org/subflow/internal/storage"
)

const testNodes = "vless://11111111-1111-1111-1111-111111111111@example.com:443" +
	"?encryption=none&security=tls&type=ws&host=a.example.com&path=%2Fws#Node-1\n" +
	"trojan://secret@example.org:443?sni=b.example.org#Node-2\n" +
	"ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@example.net:8388#Node-3\n"

type restEnvelope struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
	Error  *struct {
		Code    string `json:"code"`
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "subflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	server := New(Deps{
		Subscriptions: storage.NewSubscriptionRepo(db),
		Settings:      storage.NewSettingsRepo(db),
		Resources:     storage.NewResourceRepo(db),
		Fetcher:       fetch.New(fetch.Options{AllowPrivateHosts: true}),
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<title>SubFlow</title><div id=\"app\"></div>")},
		},
	})
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func do(t *testing.T, method, url string, body any) (int, restEnvelope, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	} else if method == http.MethodPost || method == http.MethodPatch || method == http.MethodPut {
		reader = strings.NewReader("")
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	env := restEnvelope{}
	_ = json.Unmarshal(raw, &env)
	return resp.StatusCode, env, string(raw)
}

func TestUtilEnv(t *testing.T) {
	ts := newTestServer(t)
	status, env, raw := do(t, http.MethodGet, ts.URL+"/api/utils/env", nil)
	if status != http.StatusOK || env.Status != "success" {
		t.Fatalf("env status=%d body=%s", status, raw)
	}
	var payload struct {
		Backend string          `json:"backend"`
		Feature map[string]bool `json:"feature"`
		Meta    struct {
			Node struct {
				Env map[string]string `json:"env"`
			} `json:"node"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Backend != "SubFlow" {
		t.Fatalf("backend = %q", payload.Backend)
	}
	if payload.Feature["archive"] || payload.Feature["share"] {
		t.Fatalf("feature flags should be false: %+v", payload.Feature)
	}
	if payload.Meta.Node.Env["SUB_STORE_DOCKER"] != "false" {
		t.Fatalf("missing docker env marker: %+v", payload.Meta.Node.Env)
	}
}

func TestResourceCRUD(t *testing.T) {
	ts := newTestServer(t)
	base := ts.URL + "/api"

	cases := []struct {
		create string
		item   string
		doc    map[string]any
	}{
		{"/subs", "/sub/alpha", map[string]any{"name": "alpha", "source": "local", "content": testNodes}},
		{"/collections", "/collection/alpha", map[string]any{"name": "alpha", "subscriptions": []string{}}},
		{"/files", "/file/alpha", map[string]any{"name": "alpha", "content": testNodes}},
		{"/artifacts", "/artifact/alpha", map[string]any{"name": "alpha", "type": "clash"}},
	}

	for _, tc := range cases {
		status, env, raw := do(t, http.MethodPost, base+tc.create, tc.doc)
		if status != http.StatusOK || env.Status != "success" {
			t.Fatalf("create %s: status=%d body=%s", tc.create, status, raw)
		}

		status, env, raw = do(t, http.MethodGet, base+tc.item, nil)
		if status != http.StatusOK || env.Status != "success" {
			t.Fatalf("get %s: status=%d body=%s", tc.item, status, raw)
		}

		status, _, raw = do(t, http.MethodGet, base+tc.item+"-missing", nil)
		if status != http.StatusNotFound {
			t.Fatalf("missing %s: status=%d body=%s", tc.item, status, raw)
		}

		status, env, raw = do(t, http.MethodPatch, base+tc.item, map[string]any{"displayName": "Renamed"})
		if status != http.StatusOK || env.Status != "success" {
			t.Fatalf("patch %s: status=%d body=%s", tc.item, status, raw)
		}

		status, env, raw = do(t, http.MethodDelete, base+tc.item, nil)
		if status != http.StatusOK || env.Status != "success" {
			t.Fatalf("delete %s: status=%d body=%s", tc.item, status, raw)
		}
	}
}

func TestDuplicateNameConflicts(t *testing.T) {
	ts := newTestServer(t)
	doc := map[string]any{"name": "dup", "source": "local", "content": testNodes}
	if status, _, raw := do(t, http.MethodPost, ts.URL+"/api/subs", doc); status != http.StatusOK {
		t.Fatalf("first create: %d %s", status, raw)
	}
	status, env, raw := do(t, http.MethodPost, ts.URL+"/api/subs", doc)
	if status != http.StatusConflict || env.Error == nil || env.Error.Code != "CONFLICT" {
		t.Fatalf("duplicate create: status=%d body=%s", status, raw)
	}
}

func TestPreviewAppliesProcessors(t *testing.T) {
	ts := newTestServer(t)
	doc := map[string]any{
		"name":    "preview",
		"source":  "local",
		"content": testNodes,
		"process": []map[string]any{
			{"type": "FilterProcessor", "args": map[string]any{"regex": "Node-[12]"}},
			{"type": "RenameProcessor", "args": map[string]any{"pattern": "^", "replace": "E2E "}},
		},
	}
	status, env, raw := do(t, http.MethodPost, ts.URL+"/api/preview/subs", doc)
	if status != http.StatusOK || env.Status != "success" {
		t.Fatalf("preview: status=%d body=%s", status, raw)
	}

	var payload struct {
		Original  []map[string]any `json:"original"`
		Processed []map[string]any `json:"processed"`
	}
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Original) != 3 {
		t.Fatalf("original nodes = %d, want 3", len(payload.Original))
	}
	if len(payload.Processed) != 2 {
		t.Fatalf("processed nodes = %d, want 2", len(payload.Processed))
	}
	if name, _ := payload.Processed[0]["name"].(string); name != "E2E Node-1" {
		t.Fatalf("rename not applied: %v", payload.Processed[0]["name"])
	}
}

func TestDownloadsForEveryTarget(t *testing.T) {
	ts := newTestServer(t)
	doc := map[string]any{"name": "dl", "source": "local", "content": testNodes}
	if status, _, raw := do(t, http.MethodPost, ts.URL+"/api/subs", doc); status != http.StatusOK {
		t.Fatalf("create: %d %s", status, raw)
	}

	for _, platform := range proxy.Platforms {
		status, _, raw := do(t, http.MethodGet, ts.URL+"/download/dl?target="+platform.ID, nil)
		if status != http.StatusOK {
			t.Fatalf("download target %s: status=%d body=%s", platform.ID, status, raw)
		}
		if len(strings.TrimSpace(raw)) == 0 {
			t.Fatalf("download target %s returned empty output", platform.ID)
		}
	}

	status, _, _ := do(t, http.MethodGet, ts.URL+"/download/dl?target=nope", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("unknown target status = %d, want 400", status)
	}
	status, _, _ = do(t, http.MethodGet, ts.URL+"/download/missing", nil)
	if status != http.StatusNotFound {
		t.Fatalf("missing download status = %d, want 404", status)
	}
}

func TestSettingsAndStorageRoundTrip(t *testing.T) {
	ts := newTestServer(t)

	if status, _, raw := do(t, http.MethodPatch, ts.URL+"/api/settings", map[string]any{"k": "v"}); status != http.StatusOK {
		t.Fatalf("patch settings: %d %s", status, raw)
	}
	status, env, raw := do(t, http.MethodGet, ts.URL+"/api/settings", nil)
	if status != http.StatusOK {
		t.Fatalf("get settings: %d %s", status, raw)
	}
	var settings map[string]any
	if err := json.Unmarshal(env.Data, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["k"] != "v" {
		t.Fatalf("settings did not persist: %+v", settings)
	}

	// Export then re-import into a second server. The export body is the raw
	// snapshot, and the import expects it wrapped as {content: <snapshot>}.
	status, _, exported := do(t, http.MethodGet, ts.URL+"/api/storage", nil)
	if status != http.StatusOK {
		t.Fatalf("export storage: %d", status)
	}
	if !strings.Contains(exported, `"settings"`) {
		t.Fatalf("export returned no data: %s", exported)
	}

	ts2 := newTestServer(t)
	payload := map[string]any{"content": exported}
	if status, _, raw := do(t, http.MethodPost, ts2.URL+"/api/storage", payload); status != http.StatusOK {
		t.Fatalf("import storage: %d %s", status, raw)
	}
	status, env, raw = do(t, http.MethodGet, ts2.URL+"/api/settings", nil)
	if status != http.StatusOK {
		t.Fatalf("get imported settings: %d %s", status, raw)
	}
	if err := json.Unmarshal(env.Data, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["k"] != "v" {
		t.Fatalf("imported settings missing: %+v", settings)
	}
}

func TestStorageImportReplacesStoreAndAcceptsBase64(t *testing.T) {
	ts := newTestServer(t)
	doc := map[string]any{"name": "keep", "source": "local", "content": testNodes}
	if status, _, raw := do(t, http.MethodPost, ts.URL+"/api/subs", doc); status != http.StatusOK {
		t.Fatalf("create sub: %d %s", status, raw)
	}

	// The frontend's "clear data" posts a base64 backup that only has settings.
	backup := base64.StdEncoding.EncodeToString([]byte(`{"settings":{"istabBar2":false}}`))
	status, env, raw := do(t, http.MethodPost, ts.URL+"/api/storage", map[string]any{"content": backup})
	if status != http.StatusOK || env.Status != "success" {
		t.Fatalf("import base64: %d %s", status, raw)
	}

	status, env, raw = do(t, http.MethodGet, ts.URL+"/api/subs", nil)
	if status != http.StatusOK {
		t.Fatalf("list subs: %d %s", status, raw)
	}
	var subs []map[string]any
	if err := json.Unmarshal(env.Data, &subs); err != nil {
		t.Fatal(err)
	}
	if len(subs) != 0 {
		t.Fatalf("restore should clear subs, got %d", len(subs))
	}

	status, env, raw = do(t, http.MethodGet, ts.URL+"/api/settings", nil)
	if status != http.StatusOK {
		t.Fatalf("get settings: %d %s", status, raw)
	}
	var settings map[string]any
	if err := json.Unmarshal(env.Data, &settings); err != nil {
		t.Fatal(err)
	}
	if value, ok := settings["istabBar2"]; !ok || value != false {
		t.Fatalf("settings not restored: %+v", settings)
	}
}

func TestStorageImportRejectsBackupWithoutSettings(t *testing.T) {
	ts := newTestServer(t)
	status, env, raw := do(t, http.MethodPost, ts.URL+"/api/storage", map[string]any{"content": `{"subs":{}}`})
	if status != http.StatusBadRequest || env.Error == nil || env.Error.Code != "INVALID_BACKUP_DATA" {
		t.Fatalf("expected INVALID_BACKUP_DATA, got %d %s", status, raw)
	}
}

func TestShareTokenCreateIsNotImplemented(t *testing.T) {
	ts := newTestServer(t)
	body := map[string]any{"payload": map[string]any{"type": "sub", "name": "x"}}
	status, env, raw := do(t, http.MethodPost, ts.URL+"/api/token", body)
	if status != http.StatusNotImplemented || env.Error == nil || env.Error.Code != "NOT_IMPLEMENTED" {
		t.Fatalf("expected NOT_IMPLEMENTED, got %d %s", status, raw)
	}
}

func TestArchiveDetailIsNotImplemented(t *testing.T) {
	ts := newTestServer(t)
	status, env, raw := do(t, http.MethodGet, ts.URL+"/api/archives/some-id", nil)
	if status != http.StatusNotImplemented || env.Error == nil || env.Error.Code != "NOT_IMPLEMENTED" {
		t.Fatalf("expected NOT_IMPLEMENTED, got %d %s", status, raw)
	}
}

func TestNotImplementedRoutesReturn501(t *testing.T) {
	ts := newTestServer(t)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/utils/backup"},
		{http.MethodPost, "/api/utils/age/key-pair"},
		{http.MethodPost, "/api/utils/age/public-key"},
		{http.MethodDelete, "/api/archives/abc"},
		{http.MethodPost, "/api/archives/abc/restore"},
		{http.MethodGet, "/api/sync/artifacts"},
		{http.MethodGet, "/api/sync/artifact/x"},
		{http.MethodGet, "/api/artifacts/restore"},
	}
	for _, route := range routes {
		status, env, raw := do(t, route.method, ts.URL+route.path, nil)
		if status != http.StatusNotImplemented {
			t.Fatalf("%s %s: status=%d body=%s", route.method, route.path, status, raw)
		}
		if env.Error == nil || env.Error.Code != "NOT_IMPLEMENTED" {
			t.Fatalf("%s %s: unexpected error %+v", route.method, route.path, env.Error)
		}
	}
}

func TestLogsAndArchivesEmpty(t *testing.T) {
	ts := newTestServer(t)
	status, env, raw := do(t, http.MethodGet, ts.URL+"/api/logs", nil)
	if status != http.StatusOK || env.Status != "success" {
		t.Fatalf("logs: %d %s", status, raw)
	}
	status, env, raw = do(t, http.MethodGet, ts.URL+"/api/archives", nil)
	if status != http.StatusOK || env.Status != "success" || strings.TrimSpace(string(env.Data)) != "[]" {
		t.Fatalf("archives: %d %s", status, raw)
	}
}

func TestSubFlowReportsNoFlow(t *testing.T) {
	ts := newTestServer(t)
	status, _, raw := do(t, http.MethodGet, ts.URL+"/api/sub/flow/anything", nil)
	if status != http.StatusOK || !strings.Contains(raw, "noFlow") {
		t.Fatalf("flow: %d %s", status, raw)
	}
}

func TestSPAFallbackAndUnknownAPIRoute(t *testing.T) {
	ts := newTestServer(t)
	status, _, body := do(t, http.MethodGet, ts.URL+"/subs", nil)
	if status != http.StatusOK || !strings.Contains(body, "id=\"app\"") {
		t.Fatalf("spa fallback: %d %s", status, body)
	}
	status, _, _ = do(t, http.MethodGet, ts.URL+"/api/definitely-not-real", nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown api route status = %d, want 404", status)
	}
}

func TestRemoteSubscriptionFetchAndConvert(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte(testNodes))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, payload)
	}))
	defer upstream.Close()

	ts := newTestServer(t)
	doc := map[string]any{"name": "remote", "source": "remote", "url": upstream.URL}
	if status, _, raw := do(t, http.MethodPost, ts.URL+"/api/subs", doc); status != http.StatusOK {
		t.Fatalf("create remote: %d %s", status, raw)
	}

	status, env, raw := do(t, http.MethodPost, ts.URL+"/api/preview/subs", doc)
	if status != http.StatusOK || env.Status != "success" {
		t.Fatalf("preview remote: %d %s", status, raw)
	}
	var payloadData struct {
		Original []map[string]any `json:"original"`
	}
	if err := json.Unmarshal(env.Data, &payloadData); err != nil {
		t.Fatal(err)
	}
	if len(payloadData.Original) != 3 {
		t.Fatalf("remote nodes = %d, want 3", len(payloadData.Original))
	}

	status, _, body := do(t, http.MethodGet, ts.URL+"/download/remote?target=singbox", nil)
	if status != http.StatusOK || !strings.Contains(body, "\"outbounds\"") {
		t.Fatalf("download remote: %d %s", status, body)
	}
}
