package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	srcFile, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer srcFile.Close()

	contentType := header.Header.Get("Content-Type")

	mediatype, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}

	if mediatype != "image/jpeg" && mediatype != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", err)
		return
	}

	//

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to locate video record", err)
		return
	}

	if video.UserID != userID {
		respondWithJSON(w, http.StatusUnauthorized, struct{}{})
		return
	}

	extensions, err := mime.ExtensionsByType(contentType)
	if err != nil || len(extensions) == 0 {
		respondWithError(w, http.StatusBadRequest, "Invalid content type", err)
	}
	ext := strings.TrimPrefix(extensions[0], ".")

	key := make([]byte, 32)
	rand.Read(key)
	RawURLEncoding := base64.RawURLEncoding.EncodeToString(key)
	fileName := fmt.Sprintf("%s.%s", RawURLEncoding, ext)
	filePath := filepath.Join(cfg.assetsRoot, fileName)

	dstFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error saving file", err)
	}
	defer dstFile.Close()

	io.Copy(dstFile, srcFile)

	imageURL := fmt.Sprintf("http://localhost:%s/%s", cfg.port, filePath)
	fmt.Println(imageURL)
	video.ThumbnailURL = &imageURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video record", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
