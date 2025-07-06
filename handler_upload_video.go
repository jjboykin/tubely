package main

import (
	//"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	//"strings"
	//"time"

	//"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"

	//"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxMemory = 10 << 30
	//const delimiter = ","
	//maxReader := http.MaxBytesReader(nil, r.Body, int64(maxMemory))

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

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to locate video record", err)
		return
	}

	if video.UserID != userID {
		respondWithJSON(w, http.StatusUnauthorized, struct{}{})
		return
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	r.ParseMultipartForm(maxMemory)

	srcFile, header, err := r.FormFile("video")
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

	if mediatype != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error saving temp file", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	io.Copy(tempFile, srcFile)
	tempFile.Seek(0, io.SeekStart)

	processedFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error pre-processing video file", err)
	}

	processedFile, err := os.Open(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error opening processed video file", err)
	}
	defer processedFile.Close()

	_, orientation, err := getVideoAspectRatio(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video aspect ratio", err)
	}

	key := make([]byte, 32)
	rand.Read(key)
	RawURLEncoding := base64.RawURLEncoding.EncodeToString(key)
	fileKey := fmt.Sprintf("%s/%s.mp4", orientation, RawURLEncoding)

	cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileKey,
		Body:        processedFile,
		ContentType: &contentType,
	})

	//videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fileKey)
	//videoURL := fmt.Sprintf("%s%s%s", cfg.s3Bucket, delimiter, fileKey)
	videoURL := fmt.Sprintf("%s/%s", cfg.s3CfDistribution, fileKey)
	fmt.Println(videoURL)
	video.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video record", err)
		return
	}

	/*
		video, err = cfg.dbVideoToSignedVideo(video)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Unable to locate video record", err)
			return
		}
	*/

	respondWithJSON(w, http.StatusOK, video)
}

func processVideoForFastStart(filePath string) (string, error) {
	outFilePath := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outFilePath)
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return outFilePath, nil
}

/*
func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {

	// NewPresignClient(c *Client, optFns ...func(*PresignOptions)) *PresignClient
	s3PresignClient := s3.NewPresignClient(s3Client)
	presignedRequest, err := s3PresignClient.PresignGetObject(
		context.TODO(),
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		},
		s3.WithPresignExpires(expireTime),
	)
	if err != nil {
		return "", err
	}
	return presignedRequest.URL, nil
}
*/

/*
func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	const delimiter = ","
	if video.VideoURL == nil || !strings.Contains(*video.VideoURL, delimiter) {
		return video, nil
	}
	parts := strings.Split(*video.VideoURL, delimiter)
	if len(parts) != 2 {
		return video, nil
	}
	bucket := parts[0]
	key := parts[1]
	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, time.Minute*15)
	if err != nil {
		return video, err
	}
	video.VideoURL = &presignedURL
	return video, nil
}
*/
