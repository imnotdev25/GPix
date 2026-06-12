package gpmc

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Stage int

const (
	StageHash Stage = iota
	StageDedup
	StageGetToken
	StagePut
	StageCommit
	StageDone
)

func (s Stage) String() string {
	switch s {
	case StageHash:
		return "hash"
	case StageDedup:
		return "dedup"
	case StageGetToken:
		return "get-token"
	case StagePut:
		return "put"
	case StageCommit:
		return "commit"
	case StageDone:
		return "done"
	}
	return "?"
}

type UploadResult struct {
	Path     string
	MediaKey string
	Skipped  bool
	Err      error
}

type UploadEvent struct {
	Path       string
	Stage      Stage
	BytesDone  int64
	BytesTotal int64
	Err        error
}

func (c *Client) UploadFile(ctx context.Context, path string, opts UploadOpts) (UploadResult, error) {
	return c.uploadOne(ctx, path, opts, nil)
}

func (c *Client) UploadFileWithProgress(ctx context.Context, path string, opts UploadOpts, progress func(UploadEvent)) (UploadResult, error) {
	return c.uploadOne(ctx, path, opts, progress)
}

func (c *Client) uploadOne(ctx context.Context, path string, opts UploadOpts, progress func(UploadEvent)) (UploadResult, error) {
	emit := func(s Stage, done, total int64, err error) {
		if progress != nil {
			progress(UploadEvent{Path: path, Stage: s, BytesDone: done, BytesTotal: total, Err: err})
		}
	}
	res := UploadResult{Path: path}

	st, err := os.Stat(path)
	if err != nil {
		res.Err = err
		emit(StageHash, 0, 0, err)
		return res, err
	}
	if st.IsDir() {
		res.Err = errors.New("gpmc: uploadOne called on a directory")
		return res, res.Err
	}
	size := st.Size()
	mtime := st.ModTime()
	name := filepath.Base(path)
	if opts.OverrideName != "" {
		name = opts.OverrideName
	}

	emit(StageHash, 0, size, nil)
	digest, b64, err := HashFile(path)
	if err != nil {
		res.Err = err
		emit(StageHash, 0, size, err)
		return res, err
	}

	if !opts.Force {
		emit(StageDedup, 0, size, nil)
		key, found, err := c.findRemoteMediaByHash(ctx, digest)
		if err != nil {
			res.Err = err
			emit(StageDedup, 0, size, err)
			return res, err
		}
		if found {
			res.MediaKey = key
			res.Skipped = true
			emit(StageDone, size, size, nil)
			if opts.DeleteAfter {
				_ = os.Remove(path)
			}
			return res, nil
		}
	}

	emit(StageGetToken, 0, size, nil)
	uploadID, err := c.getUploadToken(ctx, b64, size)
	if err != nil {
		res.Err = err
		emit(StageGetToken, 0, size, err)
		return res, err
	}

	emit(StagePut, 0, size, nil)
	receipt, err := c.putFile(ctx, uploadID, func() (io.ReadCloser, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		if progress == nil {
			return f, nil
		}
		return &progressReader{
			rc:    f,
			total: size,
			emit: func(done int64) {
				emit(StagePut, done, size, nil)
			},
		}, nil
	}, size)
	if err != nil {
		res.Err = err
		emit(StagePut, 0, size, err)
		return res, err
	}

	emit(StageCommit, size, size, nil)
	key, err := c.commitUpload(ctx, receipt, name, digest, opts.Quality, c.profile, mtime)
	if err != nil {
		res.Err = err
		emit(StageCommit, size, size, err)
		return res, err
	}

	res.MediaKey = key
	emit(StageDone, size, size, nil)
	if opts.DeleteAfter {
		_ = os.Remove(path)
	}
	return res, nil
}

func (c *Client) UploadFiles(ctx context.Context, paths []string, opts UploadOpts, progress func(UploadEvent)) ([]UploadResult, error) {
	expanded, err := expandPaths(paths, opts.Recursive)
	if err != nil {
		return nil, err
	}
	conc := opts.Concurrency
	if conc < 1 {
		conc = 1
	}

	results := make([]UploadResult, len(expanded))
	sem := make(chan struct{}, conc)

	var evMu sync.Mutex
	emit := func(ev UploadEvent) {
		if progress == nil {
			return
		}
		evMu.Lock()
		defer evMu.Unlock()
		progress(ev)
	}

	var wg sync.WaitGroup
	for i, p := range expanded {
		i, p := i, p
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r, _ := c.uploadOne(ctx, p, opts, emit)
			results[i] = r
		}()
	}
	wg.Wait()
	return results, nil
}

func expandPaths(paths []string, recursive bool) ([]string, error) {
	var out []string
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			out = append(out, p)
			continue
		}
		if !recursive {
			entries, err := os.ReadDir(p)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				if !e.IsDir() {
					out = append(out, filepath.Join(p, e.Name()))
				}
			}
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

var _ = time.Now
