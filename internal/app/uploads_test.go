package app

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// An upload far over the parser's default 8 KB parses with no writable temp
// directory, as on the read-only root filesystem the API runs on.
func TestUploadsParseWithoutTempDir(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	prev := humachi.MultipartMaxMemory
	t.Cleanup(func() { humachi.MultipartMaxMemory = prev })
	keepUploadsInMemory()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "test"))
	huma.Register(api, huma.Operation{OperationID: "upload", Method: http.MethodPut, Path: "/upload", MaxBodyBytes: 8 << 20},
		func(ctx context.Context, in *struct {
			RawBody huma.MultipartFormFiles[struct {
				File huma.FormFile `form:"file" required:"true"`
			}]
		}) (*struct{ Body struct{ Size int } }, error) {
			f := in.RawBody.Data().File
			defer f.Close()
			b, err := io.ReadAll(f)
			if err != nil {
				return nil, err
			}
			out := &struct{ Body struct{ Size int } }{}
			out.Body.Size = len(b)
			return out, nil
		})

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(bytes.Repeat([]byte{0x89}, 2<<20))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPut, "/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"Size":2097152`)) {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
}
