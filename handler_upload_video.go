package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
	"io"
	"mime"
	"net/http"
	"os"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxMemory = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

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

	fmt.Println("uploading video", videoID, "by user", userID)

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to find video meta", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User is not owner of video", nil)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to form video file", err)
		return
	}

	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable parse mime type", err)
		return
	}

	if mediaType != "image/jpeg" && mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Incorrect mime type, expected video .mp4", nil)
		return
	}

	tmpFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to create tmp video file", err)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to write video file", err)
		return
	}

	processedPath, err := processVideoForFastStart(tmpFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to process video file", err)
		return
	}

	processedVideo, err := os.Open(processedPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to open processed video", err)
		return
	}
	defer os.Remove(processedVideo.Name())
	defer processedVideo.Close()

	ratio, err := getVideoAspectRatio(processedVideo.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get ratio form video file", err)
		return
	}

	filePrefix := "other/"
	switch ratio {
	case Horizontal:
		filePrefix = "landscape/"
	case Vertical:
		filePrefix = "portrait/"
	}

	buf := make([]byte, 32)
	_, err = rand.Read(buf)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable create random file name", err)
		return
	}

	fileName := filePrefix + base64.RawURLEncoding.EncodeToString(buf) + ".mp4"

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileName,
		Body:        processedVideo,
		ContentType: &mediaType,
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable load file to s3", err)
		return
	}

	videoUrl := fmt.Sprintf("https://storage.yandexcloud.net/%s/%s?a=cloudfront.net", cfg.s3Bucket, fileName)
	video.VideoURL = &videoUrl

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video thumbnail", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
