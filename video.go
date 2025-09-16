package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"os/exec"
	"strings"
	"time"
)

const (
	Horizontal = "16:9"
	Vertical   = "9:16"
	Other      = "other"
)

type videoParams struct {
	Streams []struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	var vp videoParams
	err = json.Unmarshal(out.Bytes(), &vp)
	if err != nil {
		return "", err
	}

	if len(vp.Streams) < 1 {
		return "", fmt.Errorf("video %s must have at least on stream", filePath)
	}

	stream := vp.Streams[0]
	if stream.Width > stream.Height {
		if checkRightRatio(stream.Width, stream.Height) {
			return Horizontal, nil
		}
	} else {
		if checkRightRatio(stream.Height, stream.Width) {
			return Vertical, nil
		}
	}

	return Other, nil
}

func checkRightRatio(bigSide, smallSide int) bool {
	divider := float32(smallSide) / 9.0
	res := float32(bigSide) / divider
	return 15.0 <= res && res <= 17.0
}

func processVideoForFastStart(filePath string) (string, error) {
	processedFile := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", processedFile)
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return processedFile, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	s3Presign := s3.NewPresignClient(s3Client)
	req, err := s3Presign.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expireTime))

	if err != nil {
		return "", err
	}

	return req.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	parts := strings.Split(*video.VideoURL, ",")
	if len(parts) < 2 {
		return video, fmt.Errorf("there must be bucket and key in video url %s", *video.VideoURL)
	}

	presignedUrl, err := generatePresignedURL(cfg.s3Client, parts[0], parts[1], 10*time.Minute)
	if err != nil {
		return video, err
	}

	video.VideoURL = aws.String(presignedUrl)
	return video, nil
}
