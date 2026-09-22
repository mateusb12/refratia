package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func readExamMetadata(
	value map[string]any,
	key string,
) string {
	raw, ok := value[key]
	if !ok {
		return ""
	}

	result, _ := raw.(string)
	return result
}

func loadExamManifest(
	requestContext context.Context,
	client *s3.Client,
	bucket string,
	caseID string,
) (map[string]any, []byte, error) {
	key := fmt.Sprintf(
		"cases/%s/paciente_compilado.json",
		caseID,
	)

	manifestObject, getManifestObjectError := client.GetObject(
		requestContext,
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		},
	)

	if getManifestObjectError != nil {
		return nil, nil, getManifestObjectError
	}

	defer manifestObject.Body.Close()

	manifestBytes, readManifestError := io.ReadAll(
		io.LimitReader(
			manifestObject.Body,
			maxAnalysisSize,
		),
	)

	if readManifestError != nil {
		return nil, nil, readManifestError
	}

	var analysis map[string]any

	if manifestDecodeError := json.Unmarshal(manifestBytes, &analysis); manifestDecodeError != nil {
		return nil, nil, manifestDecodeError
	}

	return analysis, manifestBytes, nil
}

func routeSavedExams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listSavedExams(w, r)

	case http.MethodDelete:
		removeSavedExam(w, r)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func listSavedExams(
	w http.ResponseWriter,
	r *http.Request,
) {
	client, storageClientError := storageClient(r.Context())

	if storageClientError != nil {
		writeError(
			w,
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	presigner, storagePresignerError := storagePresigner(r.Context())

	if storagePresignerError != nil {
		writeError(
			w,
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	bucket := os.Getenv("BUCKET_NAME")

	type storedExam struct {
		CaseID       string `json:"caseId"`
		PatientName  string `json:"patientName"`
		Filename     string `json:"filename"`
		Exam         string `json:"exam,omitempty"`
		Eye          string `json:"eye,omitempty"`
		ContentType  string `json:"contentType,omitempty"`
		Path         string `json:"path"`
		SignedURL    string `json:"signedUrl,omitempty"`
		Size         int64  `json:"size"`
		LastModified string `json:"lastModified,omitempty"`
		Referenced   bool   `json:"referenced"`
	}

	exams := make([]storedExam, 0)

	var casesContinuation *string

	for {
		cases, caseListError := client.ListObjectsV2(
			r.Context(),
			&s3.ListObjectsV2Input{
				Bucket:            aws.String(bucket),
				Prefix:            aws.String("cases/"),
				Delimiter:         aws.String("/"),
				ContinuationToken: casesContinuation,
			},
		)

		if caseListError != nil {
			writeError(
				w,
				http.StatusBadGateway,
				"não foi possível listar os casos no Tigris",
			)
			return
		}

		for _, prefix := range cases.CommonPrefixes {
			casePrefix := aws.ToString(prefix.Prefix)

			caseID := strings.TrimSuffix(
				strings.TrimPrefix(
					casePrefix,
					"cases/",
				),
				"/",
			)

			if caseID == "" {
				continue
			}

			patientName := "Paciente não identificado"

			sourceByPath := make(
				map[string]map[string]any,
			)

			analysis, _, manifestReadError := loadExamManifest(
				r.Context(),
				client,
				bucket,
				caseID,
			)

			if manifestReadError == nil {
				classifySourceFilesFromExams(
					analysis,
				)

				if patient, ok := analysis["patient"].(map[string]any); ok {
					if fullName, ok := patient["full_name"].(string); ok &&
						strings.TrimSpace(fullName) != "" {
						patientName = fullName
					}
				}

				if sourceFiles, ok := analysis["source_files"].([]any); ok {
					for _, raw := range sourceFiles {
						file, ok := raw.(map[string]any)

						if !ok {
							continue
						}

						path := readExamMetadata(
							file,
							"path",
						)

						if path != "" {
							sourceByPath[path] = file
						}
					}
				}
			}

			var objectsContinuation *string

			for {
				objects, objectListError := client.ListObjectsV2(
					r.Context(),
					&s3.ListObjectsV2Input{
						Bucket:            aws.String(bucket),
						Prefix:            aws.String(casePrefix),
						ContinuationToken: objectsContinuation,
					},
				)

				if objectListError != nil {
					writeError(
						w,
						http.StatusBadGateway,
						"não foi possível listar os arquivos do caso",
					)
					return
				}

				for _, object := range objects.Contents {
					key := aws.ToString(object.Key)

					if key == "" ||
						key == casePrefix ||
						key == casePrefix+"paciente_compilado.json" {
						continue
					}

					meta, referenced := sourceByPath[key]

					exam := ""
					eye := ""
					contentType := ""

					if referenced {
						exam = readExamMetadata(
							meta,
							"exam",
						)

						eye = readExamMetadata(
							meta,
							"eye",
						)

						contentType = readExamMetadata(
							meta,
							"type",
						)
					}

					signedURL := ""

					signed, presignDownloadError := presigner.PresignGetObject(
						r.Context(),
						&s3.GetObjectInput{
							Bucket: aws.String(bucket),
							Key:    aws.String(key),
						},
					)

					if presignDownloadError == nil {
						signedURL = signed.URL
					}

					size := int64(0)

					if object.Size != nil {
						size = *object.Size
					}

					lastModified := ""

					if object.LastModified != nil {
						lastModified = object.LastModified.
							UTC().
							Format(time.RFC3339)
					}

					exams = append(
						exams,
						storedExam{
							CaseID:       caseID,
							PatientName:  patientName,
							Filename:     filepath.Base(key),
							Exam:         exam,
							Eye:          eye,
							ContentType:  contentType,
							Path:         key,
							SignedURL:    signedURL,
							Size:         size,
							LastModified: lastModified,
							Referenced:   referenced,
						},
					)
				}

				if !aws.ToBool(objects.IsTruncated) {
					break
				}

				objectsContinuation =
					objects.NextContinuationToken
			}
		}

		if !aws.ToBool(cases.IsTruncated) {
			break
		}

		casesContinuation =
			cases.NextContinuationToken
	}

	sort.Slice(
		exams,
		func(i, j int) bool {
			if exams[i].LastModified != exams[j].LastModified {
				return exams[i].LastModified >
					exams[j].LastModified
			}

			return exams[i].Filename <
				exams[j].Filename
		},
	)

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(
		map[string]any{
			"exams": exams,
		},
	)
}

func authorizeExamMutation(
	w http.ResponseWriter,
	r *http.Request,
) bool {
	token := os.Getenv("CASE_DELETE_TOKEN")

	if token == "" {
		writeError(
			w,
			http.StatusNotFound,
			"operações de escrita no storage desabilitadas",
		)
		return false
	}

	if r.Header.Get("Authorization") != "Bearer "+token {
		writeError(
			w,
			http.StatusUnauthorized,
			"não autorizado",
		)
		return false
	}

	return true
}

func prepareExamUpload(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !authorizeExamMutation(w, r) {
		return
	}

	var request struct {
		CaseID      string `json:"caseId"`
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		Size        int64  `json:"size"`
	}

	if uploadRequestDecodeError := json.NewDecoder(
		io.LimitReader(
			r.Body,
			64<<10,
		),
	).Decode(&request); uploadRequestDecodeError != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"payload inválido",
		)
		return
	}

	if !validCaseID(request.CaseID) {
		writeError(
			w,
			http.StatusBadRequest,
			"caso inválido",
		)
		return
	}

	request.Filename = filepath.Base(
		strings.TrimSpace(
			request.Filename,
		),
	)

	if request.Filename == "" ||
		request.Filename == "." {
		writeError(
			w,
			http.StatusBadRequest,
			"nome de arquivo inválido",
		)
		return
	}

	const maxDirectUploadSize = int64(20 << 20)

	if request.Size <= 0 ||
		request.Size > maxDirectUploadSize {
		writeError(
			w,
			http.StatusRequestEntityTooLarge,
			"arquivo deve ter no máximo 20 MB",
		)
		return
	}

	if !allowedTypes[request.ContentType] {
		writeError(
			w,
			http.StatusUnsupportedMediaType,
			"tipo de arquivo não suportado",
		)
		return
	}

	client, storageClientError := storageClient(r.Context())

	if storageClientError != nil {
		writeError(
			w,
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	bucket := os.Getenv("BUCKET_NAME")

	analysisKey := fmt.Sprintf(
		"cases/%s/paciente_compilado.json",
		request.CaseID,
	)

	if _, caseManifestLookupError := client.HeadObject(
		r.Context(),
		&s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(analysisKey),
		},
	); caseManifestLookupError != nil {
		writeError(
			w,
			http.StatusNotFound,
			"caso não encontrado",
		)
		return
	}

	safeFilename := safeName(
		request.Filename,
	)

	if safeFilename == "" {
		writeError(
			w,
			http.StatusBadRequest,
			"nome de arquivo inválido",
		)
		return
	}

	key := fmt.Sprintf(
		"cases/%s/%d-%s",
		request.CaseID,
		time.Now().UTC().UnixNano(),
		safeFilename,
	)

	presigner, storagePresignerError := storagePresigner(
		r.Context(),
	)

	if storagePresignerError != nil {
		writeError(
			w,
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	signed, presignUploadError := presigner.PresignPutObject(
		r.Context(),
		&s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			ContentType: aws.String(
				request.ContentType,
			),
		},
	)

	if presignUploadError != nil {
		writeError(
			w,
			http.StatusBadGateway,
			"não foi possível gerar a URL de upload",
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(
		map[string]any{
			"uploadUrl": signed.URL,
			"path":      key,
			"headers": map[string]string{
				"Content-Type": request.ContentType,
			},
		},
	)
}

func removeSavedExam(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !authorizeExamMutation(w, r) {
		return
	}

	var request struct {
		CaseID string `json:"caseId"`
		Path   string `json:"path"`
	}

	if deleteRequestDecodeError := json.NewDecoder(
		io.LimitReader(
			r.Body,
			64<<10,
		),
	).Decode(&request); deleteRequestDecodeError != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"payload inválido",
		)
		return
	}

	if !validCaseID(request.CaseID) {
		writeError(
			w,
			http.StatusBadRequest,
			"caso inválido",
		)
		return
	}

	casePrefix := fmt.Sprintf(
		"cases/%s/",
		request.CaseID,
	)

	if !strings.HasPrefix(
		request.Path,
		casePrefix,
	) ||
		request.Path ==
			casePrefix+"paciente_compilado.json" {
		writeError(
			w,
			http.StatusBadRequest,
			"arquivo inválido",
		)
		return
	}

	client, storageClientError := storageClient(r.Context())

	if storageClientError != nil {
		writeError(
			w,
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	bucket := os.Getenv("BUCKET_NAME")

	if _, examObjectLookupError := client.HeadObject(
		r.Context(),
		&s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(request.Path),
		},
	); examObjectLookupError != nil {
		writeError(
			w,
			http.StatusNotFound,
			"arquivo não encontrado",
		)
		return
	}

	analysis, originalManifest, manifestReadError :=
		loadExamManifest(
			r.Context(),
			client,
			bucket,
			request.CaseID,
		)

	analysisKey :=
		casePrefix + "paciente_compilado.json"

	manifestChanged := false
	deletedExam := ""
	deletedEye := ""

	if manifestReadError == nil {
		if sourceFiles, ok := analysis["source_files"].([]any); ok {
			filtered := make(
				[]any,
				0,
				len(sourceFiles),
			)

			for _, raw := range sourceFiles {
				file, ok := raw.(map[string]any)

				if !ok {
					filtered = append(
						filtered,
						raw,
					)
					continue
				}

				if readExamMetadata(
					file,
					"path",
				) == request.Path {
					manifestChanged = true
					deletedExam =
						readExamMetadata(
							file,
							"exam",
						)
					deletedEye =
						readExamMetadata(
							file,
							"eye",
						)
					continue
				}

				filtered = append(
					filtered,
					raw,
				)
			}

			if manifestChanged {
				analysis["source_files"] = filtered
			}
		}
	}

	if manifestChanged && deletedExam != "" {
		if exams, ok := analysis["exams"].(map[string]any); ok {
			if rawExam, exists := exams[deletedExam]; exists {
				if exam, ok := rawExam.(map[string]any); ok {
					if sources, ok := exam["source"].([]any); ok {
						filteredSources := make(
							[]any,
							0,
							len(sources),
						)

						for _, raw := range sources {
							source, _ :=
								raw.(string)

							if source != request.Path {
								filteredSources =
									append(
										filteredSources,
										raw,
									)
							}
						}

						exam["source"] =
							filteredSources
					}

					if deletedEye != "" {
						stillHasEyeSource := false

						if sourceFiles, ok :=
							analysis["source_files"].([]any); ok {
							for _, raw := range sourceFiles {
								file, ok :=
									raw.(map[string]any)

								if !ok {
									continue
								}

								if readExamMetadata(
									file,
									"exam",
								) != deletedExam {
									continue
								}

								remainingEye :=
									readExamMetadata(
										file,
										"eye",
									)

								if remainingEye ==
									deletedEye ||
									remainingEye == "AO" {
									stillHasEyeSource = true
									break
								}
							}
						}

						if !stillHasEyeSource {
							if eyes, ok :=
								exam["eyes"].(map[string]any); ok {
								if deletedEye == "AO" {
									for eye := range eyes {
										delete(
											eyes,
											eye,
										)
									}
								} else {
									delete(
										eyes,
										deletedEye,
									)
								}

								exam["eyes"] = eyes
							}
						}
					}

					sourceCount := 0

					if sources, ok :=
						exam["source"].([]any); ok {
						sourceCount = len(sources)
					}

					eyeCount := 0

					if eyes, ok :=
						exam["eyes"].(map[string]any); ok {
						eyeCount = len(eyes)
					}

					if sourceCount == 0 &&
						eyeCount == 0 {
						delete(
							exams,
							deletedExam,
						)
					} else {
						exams[deletedExam] = exam
					}
				}
			}

			analysis["exams"] = exams
		}
	}

	if manifestChanged {
		updatedManifest, manifestEncodeError :=
			json.MarshalIndent(
				analysis,
				"",
				"  ",
			)

		if manifestEncodeError != nil {
			writeError(
				w,
				http.StatusInternalServerError,
				"não foi possível atualizar o manifesto",
			)
			return
		}

		if _, manifestWriteError := client.PutObject(
			r.Context(),
			&s3.PutObjectInput{
				Bucket: aws.String(bucket),
				Key: aws.String(
					analysisKey,
				),
				Body: bytes.NewReader(
					updatedManifest,
				),
				ContentType: aws.String(
					"application/json",
				),
			},
		); manifestWriteError != nil {
			writeError(
				w,
				http.StatusBadGateway,
				"não foi possível atualizar o manifesto",
			)
			return
		}
	}

	_, deleteObjectError := client.DeleteObject(
		r.Context(),
		&s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(request.Path),
		},
	)

	if deleteObjectError != nil {
		if manifestChanged &&
			len(originalManifest) > 0 {
			_, _ = client.PutObject(
				r.Context(),
				&s3.PutObjectInput{
					Bucket: aws.String(bucket),
					Key: aws.String(
						analysisKey,
					),
					Body: bytes.NewReader(
						originalManifest,
					),
					ContentType: aws.String(
						"application/json",
					),
				},
			)
		}

		writeError(
			w,
			http.StatusBadGateway,
			"não foi possível excluir o arquivo",
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(
		map[string]any{
			"deleted":         true,
			"path":            request.Path,
			"manifestUpdated": manifestChanged,
		},
	)
}
