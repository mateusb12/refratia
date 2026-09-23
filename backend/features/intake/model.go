package intake

type FileMetadata struct {
	Filename          string `json:"filename"`
	OriginalFilename  string `json:"originalFilename,omitempty"`
	CanonicalFilename string `json:"canonicalFilename,omitempty"`
	ContentType       string `json:"contentType"`
	Size              int64  `json:"size"`
	SHA256            string `json:"sha256"`
	ExamType          string `json:"examType,omitempty"`
	Eye               string `json:"eye,omitempty"`
	Key               string `json:"key,omitempty"`
	SignedURL         string `json:"signed_url,omitempty"`
}
