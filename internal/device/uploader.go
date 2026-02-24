package device

// ImageUploader uploads image bytes to OSS and returns the public URL.
// Screenshots and other binary media should use this instead of embedding base64.
type ImageUploader interface {
	UploadImagePNG(data []byte, filename string) (url string, err error)
}
