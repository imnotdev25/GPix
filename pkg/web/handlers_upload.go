package web

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gpix/pkg/disguise"
	"gpix/pkg/gpmc"
	"gpix/pkg/mediacrypt"
)

const maxUploadBytes = 5 << 30

// uploadOne uploads one file. mtime is the file's modification time as the
// client knows it (zero if unknown); it dates the item when the file itself
// has no capture date.
func (s *Server) uploadOne(r *http.Request, src io.Reader, name string, quality gpmc.Quality, uploadID string, mtime time.Time) string {
	select {
	case s.tempSemaphore <- struct{}{}:
	case <-r.Context().Done():
		return name + "\tcanceled"
	}
	defer func() { <-s.tempSemaphore }()

	tf, err := os.CreateTemp(s.cfg.TempDir, "gpix-web-*"+filepath.Ext(name))
	if err != nil {
		return name + "\ttemp file: " + err.Error()
	}
	tmpPath := tf.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tf, src); err != nil {
		tf.Close()
		return name + "\trecv: " + err.Error()
	}
	tf.Close()

	uploadPath := tmpPath
	displayName := name
	commitName := name
	wasDisguised := false
	wasEncrypted := false

	if s.crypt != nil && s.crypt.Enabled() {
		// Encrypt the original bytes, then disguise the ciphertext as MP4 so
		// Google Photos accepts it. Google never sees the real media.
		st, serr := os.Stat(tmpPath)
		if serr != nil {
			return name + "\tstat: " + serr.Error()
		}
		encPath, err := encryptToTemp(s.cfg.TempDir, tmpPath, name, st.Size(), s.crypt)
		if err != nil {
			return name + "\tencrypt: " + err.Error()
		}
		defer os.Remove(encPath)
		wrappedPath, err := wrapToTemp(s.cfg.TempDir, encPath, name)
		if err != nil {
			return name + "\twrap: " + err.Error()
		}
		defer os.Remove(wrappedPath)
		uploadPath = wrappedPath
		commitName = name + ".mp4"
		wasDisguised = true
		wasEncrypted = true
	} else if head, err := readHead(tmpPath, 512); err == nil && disguise.ShouldWrap("", name, head) {
		wrappedPath, err := wrapToTemp(s.cfg.TempDir, tmpPath, name)
		if err != nil {
			return name + "\twrap: " + err.Error()
		}
		defer os.Remove(wrappedPath)
		uploadPath = wrappedPath
		commitName = name + ".mp4"
		wasDisguised = true
	}
	upOpts := gpmc.UploadOpts{Quality: quality, OverrideName: commitName}
	// Date it by when it was taken (EXIF / video metadata), else by the file's
	// own modification time from the client, else by the upload time.
	if dt, ok := gpmc.ExifDate(tmpPath); ok {
		upOpts.Mtime = dt
	} else if !mtime.IsZero() {
		upOpts.Mtime = mtime
	}
	res, err := s.gpc().UploadFile(r.Context(), uploadPath, upOpts)
	if err != nil {
		return displayName + "\terror: " + err.Error()
	}
	if wasDisguised && res.MediaKey != "" {
		s.thumbFromUpload(res.MediaKey, name, tmpPath)
	}
	marker := "uploaded"
	if wasDisguised {
		marker = "uploaded (disguised)"
	}
	if wasEncrypted {
		marker = "uploaded (encrypted)"
	}
	if res.Skipped {
		marker = "already in library"
	}
	return displayName + "\t" + marker + " · " + res.MediaKey
}

// thumbFromUpload renders an encrypted/disguised upload's thumbnails in the
// background from the plaintext still on disk, so it shows in the grid right
// away instead of after gpix downloads and decrypts it again. The file is
// hard-linked first because the caller deletes its copy when it returns.
func (s *Server) thumbFromUpload(mediaKey, name, plainPath string) {
	kind := thumbKindFor(name, classForExt(name))
	if kind == "" || s.thumbCached(mediaKey, thumbSizes[0], kind) {
		return
	}
	keep := plainPath + ".thumb"
	if err := os.Link(plainPath, keep); err != nil {
		return // the grid's background render will fetch it from Google instead
	}
	s.queueThumb(thumbJob{
		key:     mediaKey,
		kind:    kind,
		open:    func() (io.ReadCloser, error) { return os.Open(keep) },
		cleanup: func() { os.Remove(keep) },
	})
}

func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	read, err := io.ReadFull(f, buf)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		return buf[:read], nil
	}
	if err != nil {
		return nil, err
	}
	return buf[:read], nil
}

func encryptToTemp(tempDir, srcPath, name string, size int64, crypt *mediacrypt.Manager) (string, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	out, err := os.CreateTemp(tempDir, "gpix-enc-*")
	if err != nil {
		return "", err
	}
	defer out.Close()
	if err := crypt.Encrypt(out, src, size, name); err != nil {
		os.Remove(out.Name())
		return "", err
	}
	return out.Name(), nil
}

func wrapToTemp(tempDir, srcPath, name string) (string, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return "", err
	}
	out, err := os.CreateTemp(tempDir, "gpix-disg-*.mp4")
	if err != nil {
		return "", err
	}
	defer out.Close()
	wrapped, _ := disguise.Wrap(name, src, st.Size())
	if _, err := io.Copy(out, wrapped); err != nil {
		os.Remove(out.Name())
		return "", err
	}
	return out.Name(), nil
}

func randID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
