package gateway

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/rs/zerolog/log"
)

// OSSUploader handles Aliyun OSS uploads
type OSSUploader struct {
	client    *oss.Client
	bucket    *oss.Bucket
	domain    string
	pathPrefix string
}

// NewOSSUploader creates a new OSS uploader
func NewOSSUploader(endpoint, accessKeyID, accessKeySecret, bucket, domain, pathPrefix string) (*OSSUploader, error) {
	// Create OSS client
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create OSS client: %w", err)
	}

	// Get bucket
	b, err := client.Bucket(bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket: %w", err)
	}

	return &OSSUploader{
		client:    client,
		bucket:    b,
		domain:    domain,
		pathPrefix: pathPrefix,
	}, nil
}

// UploadResponse is the response for a successful upload
type UploadResponse struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// UploadFile uploads a file to OSS
func (u *OSSUploader) UploadFile(fileHeader *multipart.FileHeader) (*UploadResponse, error) {
	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read file content
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Generate unique filename
	ext := ""
	if parts := strings.Split(fileHeader.Filename, "."); len(parts) > 1 {
		ext = "." + parts[len(parts)-1]
	}
	randomStr := generateRandomStr(16)
	timestamp := time.Now().Format("20060102")
	filename := fmt.Sprintf("%s%s/%s%s", u.pathPrefix, timestamp, randomStr, ext)

	// Detect content type
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = detectContentType(ext)
	}

	// Upload to OSS
	options := []oss.Option{
		oss.ContentType(contentType),
		oss.ObjectACL(oss.ACLPublicRead),
	}

	err = u.bucket.PutObject(filename, bytes.NewReader(data), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to upload to OSS: %w", err)
	}

	log.Info().
		Str("filename", filename).
		Int("size", len(data)).
		Str("content_type", contentType).
		Msg("file uploaded to OSS")

	// Build public URL
	publicURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(u.domain, "/"), filename)

	return &UploadResponse{
		URL:      publicURL,
		Filename: filename,
		Size:     int64(len(data)),
	}, nil
}

// NewOSSUploaderWithConfig creates OSS uploader from config
func NewOSSUploaderWithConfig(cfg OSSConfig) (*OSSUploader, error) {
	return NewOSSUploader(
		cfg.Endpoint,
		cfg.AccessKeyID,
		cfg.AccessKeySecret,
		cfg.Bucket,
		cfg.Domain,
		cfg.PathPrefix,
	)
}

// OSSConfig is the OSS configuration
type OSSConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	Bucket          string
	Endpoint        string
	Domain          string
	PathPrefix      string
}

// detectContentType detects content type based on file extension
func detectContentType(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}

// generateRandomStr generates a random hex string
func generateRandomStr(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// UploadBytes uploads byte data to OSS
func (u *OSSUploader) UploadBytes(data []byte, filename, contentType string) (*UploadResponse, error) {
	// Add path prefix and timestamp
	timestamp := time.Now().Format("20060102")
	fullPath := fmt.Sprintf("%s%s/%s", u.pathPrefix, timestamp, filename)

	// Detect content type if not provided
	if contentType == "" {
		ext := ""
		if parts := strings.Split(filename, "."); len(parts) > 1 {
			ext = "." + parts[len(parts)-1]
		}
		contentType = detectContentType(ext)
	}

	// Upload to OSS
	options := []oss.Option{
		oss.ContentType(contentType),
		oss.ObjectACL(oss.ACLPublicRead),
	}

	err := u.bucket.PutObject(fullPath, bytes.NewReader(data), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to upload to OSS: %w", err)
	}

	log.Info().
		Str("filename", fullPath).
		Int("size", len(data)).
		Str("content_type", contentType).
		Msg("bytes uploaded to OSS")

	// Build public URL
	publicURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(u.domain, "/"), fullPath)

	return &UploadResponse{
		URL:      publicURL,
		Filename: fullPath,
		Size:     int64(len(data)),
	}, nil
}

// UploadVideoFile uploads a video file to OSS
func (u *OSSUploader) UploadVideoFile(data []byte, filename string) (*UploadResponse, error) {
	return u.UploadBytes(data, filename, "video/mp4")
}

// UploadImageFile uploads an image file to OSS
func (u *OSSUploader) UploadImageFile(data []byte, filename string) (*UploadResponse, error) {
	return u.UploadBytes(data, filename, "image/jpeg")
}

// DownloadAndUploadVideo downloads a video from URL and uploads it to OSS
// Returns the OSS URL and a channel that signals completion
func (u *OSSUploader) DownloadAndUploadVideo(videoURL string) (string, <-chan *UploadResult) {
	resultChan := make(chan *UploadResult, 1)

	go func() {
		defer close(resultChan)

		log.Info().Str("source_url", videoURL).Msg("starting video download from DashScope")

		// Download video from URL
		client := &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		}

		resp, err := client.Get(videoURL)
		if err != nil {
			log.Error().Err(err).Str("url", videoURL).Msg("failed to download video")
			resultChan <- &UploadResult{Error: err.Error()}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("download failed with status %d", resp.StatusCode)
			log.Error().Err(err).Str("url", videoURL).Int("status", resp.StatusCode).Msg("failed to download video")
			resultChan <- &UploadResult{Error: err.Error()}
			return
		}

		// Read video data
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error().Err(err).Str("url", videoURL).Msg("failed to read video data")
			resultChan <- &UploadResult{Error: err.Error()}
			return
		}

		log.Info().
			Str("source_url", videoURL).
			Int("size_bytes", len(data)).
			Msg("video downloaded successfully, uploading to OSS")

		// Generate filename
		timestamp := time.Now().Format("20060102_150405")
		filename := fmt.Sprintf("videos/%s.mp4", timestamp)

		// Upload to OSS
		uploadResp, err := u.UploadVideoFile(data, filename)
		if err != nil {
			log.Error().Err(err).Str("filename", filename).Msg("failed to upload video to OSS")
			resultChan <- &UploadResult{Error: err.Error()}
			return
		}

		log.Info().
			Str("oss_url", uploadResp.URL).
			Str("filename", uploadResp.Filename).
			Int64("size", uploadResp.Size).
			Msg("video uploaded to OSS successfully")

		resultChan <- &UploadResult{
			URL:      uploadResp.URL,
			Filename: uploadResp.Filename,
			Size:     uploadResp.Size,
		}
	}()

	// Return a placeholder URL immediately - will be updated when upload completes
	return videoURL, resultChan
}

// UploadResult represents the result of an async upload operation
type UploadResult struct {
	URL      string
	Filename string
	Size     int64
	Error    string
}
