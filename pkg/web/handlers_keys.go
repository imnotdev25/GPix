package web

import (
	"io"
	"net/http"
)

// handleEncryptionKeyUpload accepts one or more uploaded key files and adds each
// as an additional decryption key (saved as encryption_<id>.key). gpix then
// tries every loaded key when decrypting.
func (s *Server) handleEncryptionKeyUpload(w http.ResponseWriter, r *http.Request) {
	if s.crypt == nil {
		http.Error(w, "encryption unavailable", http.StatusNotFound)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // keys are tiny
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "part: "+err.Error(), http.StatusBadRequest)
			return
		}
		if part.FormName() != "keys" || part.FileName() == "" {
			_, _ = io.Copy(io.Discard, part)
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(part, 8192))
		name, aerr := s.crypt.AddKeyRaw(raw)
		switch {
		case aerr != nil:
			s.log.Warn("encryption key upload rejected", "file", part.FileName(), "err", aerr)
		case name != "":
			s.log.Info("encryption key added", "as", name)
		}
	}
	http.Redirect(w, r, "/settings/gateways", http.StatusSeeOther)
}
