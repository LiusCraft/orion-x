package assets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/storage"
	"github.com/liuscraft/orion-x/internal/store"
)

// ── 测试替身（沿用仓库约定：内联 mock，不引入 mock 生成器）──

type fakeStore struct {
	byID      map[string]*store.Asset
	created   []*store.Asset
	deleted   []string
	createErr error
}

func newFakeStore() *fakeStore { return &fakeStore{byID: map[string]*store.Asset{}} }

func (f *fakeStore) Create(a *store.Asset) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.byID[a.ID] = a
	f.created = append(f.created, a)
	return nil
}

func (f *fakeStore) GetByID(id string) (*store.Asset, error) {
	a, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return a, nil
}

func (f *fakeStore) List(ownerID, purpose, nameQuery string, _, _ int) ([]store.Asset, int64, error) {
	var out []store.Asset
	for _, a := range f.byID {
		if a.OwnerID != ownerID {
			continue
		}
		if purpose != "" && a.Purpose != purpose {
			continue
		}
		if nameQuery != "" && !strings.Contains(a.Name, nameQuery) {
			continue
		}
		out = append(out, *a)
	}
	return out, int64(len(out)), nil
}

func (f *fakeStore) DeleteByID(id string) error {
	f.deleted = append(f.deleted, id)
	delete(f.byID, id)
	return nil
}

type fakeStorage struct {
	objects        map[string][]byte
	putErr         error
	deleteErr      error
	lastKey        string
	lastTTL        time.Duration
	lastPutOptions storage.PutOptions
	lastPresign    storage.PresignOptions
}

func newFakeStorage() *fakeStorage { return &fakeStorage{objects: map[string][]byte{}} }

func (f *fakeStorage) Put(_ context.Context, key string, body io.Reader, opts storage.PutOptions) error {
	f.lastKey, f.lastPutOptions = key, opts
	if f.putErr != nil {
		return f.putErr
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	f.objects[key] = data
	return nil
}

func (f *fakeStorage) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, storage.ErrNotFound
}

func (f *fakeStorage) Delete(_ context.Context, key string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.objects, key)
	return nil
}

func (f *fakeStorage) PresignGet(_ context.Context, key string, ttl time.Duration, opts storage.PresignOptions) (string, error) {
	f.lastTTL, f.lastPresign = ttl, opts
	return "https://storage.test/" + key + "?signature=stub", nil
}

func (f *fakeStorage) Ping(context.Context) error { return nil }

func newTestService(st assetStore, be storage.Storage) *Service {
	return NewService(st, be, storage.Config{Prefix: "test", PresignTTL: time.Minute})
}

// pngBytes 最小合法 PNG 文件头。
func pngBytes() []byte { return []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR") }

// wavBytes 最小 RIFF/WAVE 文件头。
func wavBytes() []byte { return []byte("RIFF\x24\x00\x00\x00WAVEfmt ") }

// ── Create ──

func TestCreateValidImage(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	svc := newTestService(st, be)
	body := pngBytes()

	asset, err := svc.Create(context.Background(), Upload{
		OwnerID:  "user-1",
		Purpose:  PurposeImage,
		FileName: "Logo.PNG",
		Size:     int64(len(body)),
		Body:     bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if asset.MimeType != "image/png" {
		t.Fatalf("mime = %q, want image/png", asset.MimeType)
	}
	if asset.Name != "Logo.PNG" {
		t.Fatalf("name = %q, want Logo.PNG", asset.Name)
	}
	if asset.Creator != "user-1" {
		t.Fatalf("creator = %q", asset.Creator)
	}
	wantKey := fmt.Sprintf("test/image/user-1/%s.png", asset.ID)
	if asset.ObjectKey != wantKey {
		t.Fatalf("key = %q, want %q", asset.ObjectKey, wantKey)
	}
	if got := be.objects[asset.ObjectKey]; !bytes.Equal(got, body) {
		t.Fatal("object body not uploaded")
	}
	if be.lastPutOptions.ContentType != "image/png" || be.lastPutOptions.Size != int64(len(body)) {
		t.Fatalf("put options = %+v", be.lastPutOptions)
	}
	if len(st.created) != 1 {
		t.Fatalf("created records = %d, want 1", len(st.created))
	}
}

func TestCreateValidWavVoiceSample(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	svc := newTestService(st, be)
	body := wavBytes()

	asset, err := svc.Create(context.Background(), Upload{
		OwnerID: "user-1", Purpose: PurposeVoiceSample, FileName: "sample.wav",
		Size: int64(len(body)), Body: bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("create wav: %v", err)
	}
	if !strings.HasPrefix(asset.ObjectKey, "test/voice_sample/user-1/") {
		t.Fatalf("key = %q", asset.ObjectKey)
	}
}

func TestCreateRejectsInvalid(t *testing.T) {
	cases := []struct {
		name   string
		upload Upload
	}{
		{
			name: "unknown purpose",
			upload: Upload{OwnerID: "u", Purpose: Purpose("bogus"), FileName: "a.png",
				Size: int64(len(pngBytes())), Body: bytes.NewReader(pngBytes())},
		},
		{
			name: "empty file",
			upload: Upload{OwnerID: "u", Purpose: PurposeImage, FileName: "a.png",
				Size: 0, Body: bytes.NewReader(nil)},
		},
		{
			name: "missing owner",
			upload: Upload{Purpose: PurposeImage, FileName: "a.png",
				Size: int64(len(pngBytes())), Body: bytes.NewReader(pngBytes())},
		},
		{
			name: "oversize image",
			upload: Upload{OwnerID: "u", Purpose: PurposeImage, FileName: "a.png",
				Size: imageMaxSize + 1, Body: bytes.NewReader(pngBytes())},
		},
		{
			name: "unsupported extension",
			upload: Upload{OwnerID: "u", Purpose: PurposeImage, FileName: "a.exe",
				Size: int64(len(pngBytes())), Body: bytes.NewReader(pngBytes())},
		},
		{
			name: "image extension with html content",
			upload: Upload{OwnerID: "u", Purpose: PurposeImage, FileName: "a.png",
				Size: 30, Body: bytes.NewReader([]byte("<html><body>hi</body></html>"))},
		},
		{
			name: "voice extension with text content",
			upload: Upload{OwnerID: "u", Purpose: PurposeVoiceSample, FileName: "a.wav",
				Size: 11, Body: bytes.NewReader([]byte("hello world"))},
		},
		{
			name: "binary upload as document",
			upload: Upload{OwnerID: "u", Purpose: PurposeKBDocument, FileName: "a.exe",
				Size: 4, Body: bytes.NewReader([]byte("\x7fELF"))},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, be := newFakeStore(), newFakeStorage()
			svc := newTestService(st, be)

			_, err := svc.Create(context.Background(), tc.upload)
			var invalidErr *ValidationError
			if !errors.As(err, &invalidErr) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if invalidErr.Reason == "" {
				t.Fatal("validation error must carry a user-facing reason")
			}
			if len(st.created) != 0 || len(be.objects) != 0 {
				t.Fatal("rejected upload must not touch storage or db")
			}
		})
	}
}

func TestCreateStorageFailureLeavesNoRecord(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	be.putErr = errors.New("boom")
	svc := newTestService(st, be)

	_, err := svc.Create(context.Background(), Upload{
		OwnerID: "u", Purpose: PurposeImage, FileName: "a.png",
		Size: int64(len(pngBytes())), Body: bytes.NewReader(pngBytes()),
	})
	if !errors.Is(err, ErrStorage) {
		t.Fatalf("err = %v, want ErrStorage", err)
	}
	if len(st.created) != 0 {
		t.Fatal("failed upload must not create a record")
	}
}

func TestCreateRecordFailureRollsBackObject(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	st.createErr = errors.New("db down")
	svc := newTestService(st, be)

	_, err := svc.Create(context.Background(), Upload{
		OwnerID: "u", Purpose: PurposeImage, FileName: "a.png",
		Size: int64(len(pngBytes())), Body: bytes.NewReader(pngBytes()),
	})
	if err == nil {
		t.Fatal("want error")
	}
	if len(be.objects) != 0 {
		t.Fatalf("orphan object left behind: %v", be.objects)
	}
}

func TestCreateRejectsUnseekableBody(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	svc := newTestService(st, be)

	body := io.NopCloser(bytes.NewReader(pngBytes()))
	_, err := svc.Create(context.Background(), Upload{
		OwnerID: "u", Purpose: PurposeImage, FileName: "a.png",
		Size: int64(len(pngBytes())), Body: body,
	})
	var invalidErr *ValidationError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}

// ── Get / Delete ──

func TestGetEnforcesOwnership(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	svc := newTestService(st, be)
	st.byID["a1"] = &store.Asset{ID: "a1", OwnerID: "owner", Purpose: string(PurposeImage), ObjectKey: "k"}

	if _, err := svc.Get(context.Background(), "other", "a1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if _, err := svc.Get(context.Background(), "owner", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(context.Background(), "owner", "a1"); err != nil {
		t.Fatalf("owner get: %v", err)
	}
}

func TestDeleteOnlyAllowsImages(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	svc := newTestService(st, be)
	st.byID["doc"] = &store.Asset{ID: "doc", OwnerID: "u", Purpose: string(PurposeKBDocument), ObjectKey: "kb"}
	be.objects["kb"] = []byte("x")

	if err := svc.Delete(context.Background(), "u", "doc"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if _, ok := be.objects["kb"]; !ok {
		t.Fatal("domain asset must not be deleted from here")
	}

	st.byID["img"] = &store.Asset{ID: "img", OwnerID: "u", Purpose: string(PurposeImage), ObjectKey: "ik"}
	be.objects["ik"] = []byte("y")
	if err := svc.Delete(context.Background(), "u", "img"); err != nil {
		t.Fatalf("delete image: %v", err)
	}
	if _, ok := be.objects["ik"]; ok {
		t.Fatal("object not deleted")
	}
	if len(st.deleted) != 1 || st.deleted[0] != "img" {
		t.Fatalf("deleted = %v, want [img]", st.deleted)
	}
}

func TestDeleteKeepsRecordWhenStorageFails(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	be.deleteErr = errors.New("boom")
	svc := newTestService(st, be)
	st.byID["img"] = &store.Asset{ID: "img", OwnerID: "u", Purpose: string(PurposeImage), ObjectKey: "ik"}

	if err := svc.Delete(context.Background(), "u", "img"); !errors.Is(err, ErrStorage) {
		t.Fatalf("err = %v, want ErrStorage", err)
	}
	if len(st.deleted) != 0 {
		t.Fatal("record must survive a failed object delete so the user can retry")
	}
}

// TestDeleteCascade 领域级联删除：音色删除时清掉参考音频。
func TestDeleteCascade(t *testing.T) {
	t.Run("matching purpose removes object and record", func(t *testing.T) {
		st, be := newFakeStore(), newFakeStorage()
		svc := newTestService(st, be)
		st.byID["v1"] = &store.Asset{ID: "v1", OwnerID: "u", Purpose: string(PurposeVoiceSample), ObjectKey: "vk"}
		be.objects["vk"] = []byte("audio")

		if err := svc.DeleteCascade(context.Background(), "u", "v1", PurposeVoiceSample); err != nil {
			t.Fatalf("delete cascade: %v", err)
		}
		if _, ok := be.objects["vk"]; ok {
			t.Fatal("object not deleted")
		}
		if len(st.deleted) != 1 || st.deleted[0] != "v1" {
			t.Fatalf("deleted = %v, want [v1]", st.deleted)
		}
	})

	t.Run("purpose mismatch keeps the asset", func(t *testing.T) {
		st, be := newFakeStore(), newFakeStorage()
		svc := newTestService(st, be)
		st.byID["doc"] = &store.Asset{ID: "doc", OwnerID: "u", Purpose: string(PurposeKBDocument), ObjectKey: "dk"}
		be.objects["dk"] = []byte("doc")

		if err := svc.DeleteCascade(context.Background(), "u", "doc", PurposeVoiceSample); !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
		if _, ok := be.objects["dk"]; !ok {
			t.Fatal("mismatched asset must not be deleted")
		}
	})

	t.Run("missing record is treated as success", func(t *testing.T) {
		st, be := newFakeStore(), newFakeStorage()
		svc := newTestService(st, be)

		if err := svc.DeleteCascade(context.Background(), "u", "gone", PurposeVoiceSample); err != nil {
			t.Fatalf("err = %v, want nil for idempotent retry", err)
		}
	})

	t.Run("foreign asset is forbidden", func(t *testing.T) {
		st, be := newFakeStore(), newFakeStorage()
		svc := newTestService(st, be)
		st.byID["v1"] = &store.Asset{ID: "v1", OwnerID: "other", Purpose: string(PurposeVoiceSample), ObjectKey: "vk"}

		if err := svc.DeleteCascade(context.Background(), "u", "v1", PurposeVoiceSample); !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("storage failure keeps record for retry", func(t *testing.T) {
		st, be := newFakeStore(), newFakeStorage()
		be.deleteErr = errors.New("boom")
		svc := newTestService(st, be)
		st.byID["v1"] = &store.Asset{ID: "v1", OwnerID: "u", Purpose: string(PurposeVoiceSample), ObjectKey: "vk"}

		if err := svc.DeleteCascade(context.Background(), "u", "v1", PurposeVoiceSample); !errors.Is(err, ErrStorage) {
			t.Fatalf("err = %v, want ErrStorage", err)
		}
		if len(st.deleted) != 0 {
			t.Fatal("record must survive a failed object delete")
		}
	})
}

// ── List / Presign ──

func TestListNormalize(t *testing.T) {
	cases := []struct {
		in       ListQuery
		page     int
		pageSize int
	}{
		{in: ListQuery{}, page: 1, pageSize: defaultPageSize},
		{in: ListQuery{Page: -3, PageSize: -1}, page: 1, pageSize: defaultPageSize},
		{in: ListQuery{Page: 2, PageSize: 1000}, page: 2, pageSize: maxPageSize},
		{in: ListQuery{Page: 3, PageSize: 50}, page: 3, pageSize: 50},
	}
	for _, tc := range cases {
		got := tc.in.Normalize()
		if got.Page != tc.page || got.PageSize != tc.pageSize {
			t.Fatalf("normalize(%+v) = page %d size %d, want %d/%d", tc.in, got.Page, got.PageSize, tc.page, tc.pageSize)
		}
	}
}

func TestListFiltersAndRejectsUnknownPurpose(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	svc := newTestService(st, be)
	st.byID["i1"] = &store.Asset{ID: "i1", OwnerID: "u", Purpose: string(PurposeImage), Name: "logo.png"}
	st.byID["d1"] = &store.Asset{ID: "d1", OwnerID: "u", Purpose: string(PurposeKBDocument), Name: "手册.md"}
	st.byID["other"] = &store.Asset{ID: "other", OwnerID: "someone-else", Purpose: string(PurposeImage), Name: "logo.png"}

	list, total, err := svc.List(context.Background(), ListQuery{OwnerID: "u"})
	if err != nil || total != 2 || len(list) != 2 {
		t.Fatalf("list = %d/%d err=%v, want 2", len(list), total, err)
	}
	list, total, err = svc.List(context.Background(), ListQuery{OwnerID: "u", Purpose: PurposeImage})
	if err != nil || total != 1 || list[0].ID != "i1" {
		t.Fatalf("filtered list = %v total=%d err=%v", list, total, err)
	}
	list, total, err = svc.List(context.Background(), ListQuery{OwnerID: "u", Name: "手册"})
	if err != nil || total != 1 || list[0].ID != "d1" {
		t.Fatalf("name filter = %v total=%d err=%v", list, total, err)
	}
	if _, _, err := svc.List(context.Background(), ListQuery{OwnerID: "u", Purpose: Purpose("bogus")}); err == nil {
		t.Fatal("want error for unknown purpose filter")
	}
}

func TestPresignURLAndView(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	svc := newTestService(st, be)
	asset := &store.Asset{
		ID: "a1", OwnerID: "u", Purpose: string(PurposeImage),
		ObjectKey: "test/image/u/a1.png", Name: "中文 名称.png",
	}

	url, ttl, err := svc.PresignURL(context.Background(), asset, true)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	if !strings.Contains(url, asset.ObjectKey) {
		t.Fatalf("url = %q, want key inside", url)
	}
	if ttl != time.Minute {
		t.Fatalf("ttl = %v, want 1m", ttl)
	}
	if be.lastTTL != time.Minute {
		t.Fatalf("signed ttl = %v, want 1m", be.lastTTL)
	}
	if be.lastPresign.DownloadFileName != asset.Name {
		t.Fatalf("download name = %q, want %q", be.lastPresign.DownloadFileName, asset.Name)
	}

	view, err := svc.View(context.Background(), asset)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.URL == "" || view.ID != asset.ID {
		t.Fatalf("view = %+v", view)
	}
	if be.lastPresign.DownloadFileName != "" {
		t.Fatal("preview URL must not force a download")
	}

	views, err := svc.Views(context.Background(), []store.Asset{*asset})
	if err != nil || len(views) != 1 {
		t.Fatalf("views = %v err=%v", views, err)
	}
}

// TestPresignURLWithTTL 消费方（厂商拉取参考音频）需要比默认 presign_ttl 更长的窗口。
func TestPresignURLWithTTL(t *testing.T) {
	st, be := newFakeStore(), newFakeStorage()
	svc := newTestService(st, be)
	asset := &store.Asset{ID: "a1", OwnerID: "u", Purpose: string(PurposeVoiceSample), ObjectKey: "test/voice_sample/u/a1.wav"}

	url, err := svc.PresignURLWithTTL(context.Background(), asset, 2*time.Hour, false)
	if err != nil {
		t.Fatalf("presign with ttl: %v", err)
	}
	if !strings.Contains(url, asset.ObjectKey) {
		t.Fatalf("url = %q, want key inside", url)
	}
	if be.lastTTL != 2*time.Hour {
		t.Fatalf("signed ttl = %v, want 2h", be.lastTTL)
	}
	if be.lastPresign.DownloadFileName != "" {
		t.Fatal("vendor URL must stay inline, not an attachment")
	}
}

// ── 纯函数 ──

func TestSanitizeName(t *testing.T) {
	long := strings.Repeat("中", 500)
	cases := []struct {
		in   string
		want string
	}{
		{in: "a.png", want: "a.png"},
		{in: "../../etc/passwd", want: "passwd"},
		{in: `C:\Users\me\logo.png`, want: "logo.png"},
		{in: "  ", want: "未命名文件"},
		{in: "..", want: "未命名文件"},
	}
	for _, tc := range cases {
		if got := sanitizeName(tc.in); got != tc.want {
			t.Fatalf("sanitizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := []rune(sanitizeName(long)); len(got) != maxNameRunes {
		t.Fatalf("long name length = %d, want %d", len(got), maxNameRunes)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{in: 512, want: "512B"},
		{in: 2048, want: "2KB"},
		{in: 5 << 20, want: "5MB"},
	}
	for _, tc := range cases {
		if got := humanSize(tc.in); got != tc.want {
			t.Fatalf("humanSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDocumentExtensionsMatchParser(t *testing.T) {
	sp, ok := purposeSpecs[PurposeKBDocument]
	if !ok {
		t.Fatal("kb_document spec missing")
	}
	if _, ok := sp.exts[".url"]; ok {
		t.Fatal(".url is a virtual extension and must not be uploadable")
	}
	for _, ext := range []string{".txt", ".md", ".json", ".csv"} {
		if _, ok := sp.exts[ext]; !ok {
			t.Fatalf("%s must be allowed for kb_document", ext)
		}
	}
}
