package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
	"io"
	"mime"
	"net/http"
	"os"
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
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse multipart form", err)
		return
	}

	// "thumbnail" should match the HTML form input name
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to find video meta", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User is not owner of video", nil)
		return
	}

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable parse mime type", err)
		return
	}

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Incorrect mime type, expected image", nil)
		return
	}

	buf := make([]byte, 32)
	_, err = rand.Read(buf)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable create random file name", err)
		return
	}

	randName := base64.RawURLEncoding.EncodeToString(buf)
	thName := getAssetPath(randName, mediaType)
	thPath := cfg.getAssetDiskPath(thName)

	newFile, err := os.Create(thPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create thumbnail file", err)
		return
	}

	defer newFile.Close()

	_, err = io.Copy(newFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to write thumbnail file", err)
		return
	}

	thUrl := cfg.getAssetURL(thName)
	video.ThumbnailURL = &thUrl

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video thumbnail", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
