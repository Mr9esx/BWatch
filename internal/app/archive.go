package app

import (
	"archive/zip"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

func (a *App) serveArchive(w http.ResponseWriter, r *http.Request) {
	for _, task := range a.Snapshot().Tasks {
		if task.ID != r.PathValue("id") || task.Kind != "dynamic" || task.Status != "completed" {
			continue
		}
		paths := append([]string{task.File}, task.Files...)
		paths = append(paths, filepath.Join(filepath.Dir(task.File), "metadata.json"))
		verified := make([]string, 0, len(paths))
		for _, path := range paths {
			safe, err := a.safeFile(path)
			if err != nil {
				http.Error(w, "部分归档文件已移动或不存在", 404)
				return
			}
			verified = append(verified, safe)
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape("BiliFetch-"+task.VideoID+".zip"))
		archive := zip.NewWriter(w)
		defer archive.Close()
		for _, path := range verified {
			file, err := os.Open(path)
			if err != nil {
				return
			}
			method := uint16(zip.Store)
			if filepath.Ext(path) == ".md" || filepath.Ext(path) == ".json" {
				method = zip.Deflate
			}
			entry, err := archive.CreateHeader(&zip.FileHeader{Name: filepath.Base(path), Method: method})
			if err != nil {
				file.Close()
				return
			}
			_, err = io.Copy(entry, file)
			file.Close()
			if err != nil {
				return
			}
		}
		return
	}
	http.NotFound(w, r)
}
