package exams

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

	"refratia/backend/src/features/cases"
	"refratia/backend/src/shared/fileStorage"
	"refratia/backend/src/shared/httpresponse"
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
	client fileStorage.Repository,
	bucket string,
	caseID string,
) (map[string]any, []byte, error) {
	key := fmt.Sprintf(
		fileStorage.CasesPrefix+"%s/paciente_compilado.json",
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
			fileStorage.MaxAnalysisSize,
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

func (handler Handler) ServeExams(responseWriter http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		listSavedExams(responseWriter, request)

	case http.MethodDelete:
		handler.removeSavedExam(responseWriter, request)

	default:
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (handler Handler) ServeDownload(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	caseID := request.URL.Query().Get("caseId")
	path := request.URL.Query().Get("path")
	casePrefix := fmt.Sprintf(fileStorage.CasesPrefix+"%s/", caseID)

	if !cases.ValidID(caseID) ||
		!strings.HasPrefix(path, casePrefix) ||
		path == casePrefix+"paciente_compilado.json" {
		httpresponse.WriteError(responseWriter, http.StatusBadRequest, "arquivo inválido")
		return
	}

	client, storageClientError := fileStorage.New(request.Context())
	if storageClientError != nil {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "storage indisponível")
		return
	}

	object, downloadError := client.GetObject(
		request.Context(),
		&s3.GetObjectInput{
			Bucket: aws.String(os.Getenv("BUCKET_NAME")),
			Key:    aws.String(path),
		},
	)
	if downloadError != nil {
		httpresponse.WriteError(responseWriter, http.StatusNotFound, "arquivo não encontrado")
		return
	}
	defer object.Body.Close()

	contentType := aws.ToString(object.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	filename := strings.NewReplacer("\"", "", "\r", "", "\n", "").Replace(filepath.Base(path))
	responseWriter.Header().Set("Content-Type", contentType)
	responseWriter.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	if object.ContentLength != nil {
		responseWriter.Header().Set("Content-Length", fmt.Sprintf("%d", *object.ContentLength))
	}

	if _, copyError := io.Copy(responseWriter, object.Body); copyError != nil {
		return
	}
}

func listSavedExams(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	client, storageClientError := fileStorage.New(request.Context())

	if storageClientError != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	presigner, storagePresignerError := fileStorage.NewPresigner(request.Context())

	if storagePresignerError != nil {
		httpresponse.WriteError(
			responseWriter,
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
			request.Context(),
			&s3.ListObjectsV2Input{
				Bucket:            aws.String(bucket),
				Prefix:            aws.String(fileStorage.CasesPrefix),
				Delimiter:         aws.String("/"),
				ContinuationToken: casesContinuation,
			},
		)

		if caseListError != nil {
			httpresponse.WriteError(
				responseWriter,
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
					fileStorage.CasesPrefix,
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
				request.Context(),
				client,
				bucket,
				caseID,
			)

			if manifestReadError == nil {
				ClassifySourceFiles(
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
					request.Context(),
					&s3.ListObjectsV2Input{
						Bucket:            aws.String(bucket),
						Prefix:            aws.String(casePrefix),
						ContinuationToken: objectsContinuation,
					},
				)

				if objectListError != nil {
					httpresponse.WriteError(
						responseWriter,
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
						request.Context(),
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
		func(left, right int) bool {
			if exams[left].LastModified != exams[right].LastModified {
				return exams[left].LastModified >
					exams[right].LastModified
			}

			return exams[left].Filename <
				exams[right].Filename
		},
	)

	responseWriter.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(responseWriter).Encode(
		map[string]any{
			"exams": exams,
		},
	)
}

func authorizeExamMutation(
	responseWriter http.ResponseWriter,
	request *http.Request,
) bool {
	token := os.Getenv("CASE_DELETE_TOKEN")

	if token == "" {
		httpresponse.WriteError(
			responseWriter,
			http.StatusNotFound,
			"operações de escrita no storage desabilitadas",
		)
		return false
	}

	if request.Header.Get("Authorization") != "Bearer "+token {
		httpresponse.WriteError(
			responseWriter,
			http.StatusUnauthorized,
			"não autorizado",
		)
		return false
	}

	return true
}

func (handler Handler) ServeUploadURL(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !authorizeExamMutation(responseWriter, request) {
		return
	}

	var payload struct {
		CaseID      string `json:"caseId"`
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		Size        int64  `json:"size"`
	}

	if uploadRequestDecodeError := json.NewDecoder(
		io.LimitReader(
			request.Body,
			64<<10,
		),
	).Decode(&payload); uploadRequestDecodeError != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"payload inválido",
		)
		return
	}

	if !cases.ValidID(payload.CaseID) {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"caso inválido",
		)
		return
	}

	payload.Filename = filepath.Base(
		strings.TrimSpace(
			payload.Filename,
		),
	)

	if payload.Filename == "" ||
		payload.Filename == "." {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"nome de arquivo inválido",
		)
		return
	}

	const maxDirectUploadSize = int64(20 << 20)

	if payload.Size <= 0 ||
		payload.Size > maxDirectUploadSize {
		httpresponse.WriteError(
			responseWriter,
			http.StatusRequestEntityTooLarge,
			"arquivo deve ter no máximo 20 MB",
		)
		return
	}

	if !AllowedUploadTypes[payload.ContentType] {
		httpresponse.WriteError(
			responseWriter,
			http.StatusUnsupportedMediaType,
			"tipo de arquivo não suportado",
		)
		return
	}

	client, storageClientError := fileStorage.New(request.Context())

	if storageClientError != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	bucket := os.Getenv("BUCKET_NAME")

	analysisKey := fmt.Sprintf(
		fileStorage.CasesPrefix+"%s/paciente_compilado.json",
		payload.CaseID,
	)

	if _, caseManifestLookupError := client.HeadObject(
		request.Context(),
		&s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(analysisKey),
		},
	); caseManifestLookupError != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusNotFound,
			"caso não encontrado",
		)
		return
	}

	safeFilename := safeName(
		payload.Filename,
	)

	if safeFilename == "" {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"nome de arquivo inválido",
		)
		return
	}

	key := fmt.Sprintf(
		fileStorage.CasesPrefix+"%s/%d-%s",
		payload.CaseID,
		time.Now().UTC().UnixNano(),
		safeFilename,
	)

	presigner, storagePresignerError := fileStorage.NewPresigner(
		request.Context(),
	)

	if storagePresignerError != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	signed, presignUploadError := presigner.PresignPutObject(
		request.Context(),
		&s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			ContentType: aws.String(
				payload.ContentType,
			),
		},
	)

	if presignUploadError != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadGateway,
			"não foi possível gerar a URL de upload",
		)
		return
	}

	responseWriter.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(responseWriter).Encode(
		map[string]any{
			"uploadUrl": signed.URL,
			"path":      key,
			"headers": map[string]string{
				"Content-Type": payload.ContentType,
			},
		},
	)
}

func (handler Handler) removeSavedExam(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	if !authorizeExamMutation(responseWriter, request) {
		return
	}

	var payload struct {
		CaseID string `json:"caseId"`
		Path   string `json:"path"`
	}

	if deleteRequestDecodeError := json.NewDecoder(
		io.LimitReader(
			request.Body,
			64<<10,
		),
	).Decode(&payload); deleteRequestDecodeError != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"payload inválido",
		)
		return
	}

	if !cases.ValidID(payload.CaseID) {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"caso inválido",
		)
		return
	}

	casePrefix := fmt.Sprintf(
		fileStorage.CasesPrefix+"%s/",
		payload.CaseID,
	)

	if !strings.HasPrefix(
		payload.Path,
		casePrefix,
	) ||
		payload.Path ==
			casePrefix+"paciente_compilado.json" {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"arquivo inválido",
		)
		return
	}

	client, storageClientError := fileStorage.New(request.Context())

	if storageClientError != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	bucket := os.Getenv("BUCKET_NAME")

	if _, examObjectLookupError := client.HeadObject(
		request.Context(),
		&s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(payload.Path),
		},
	); examObjectLookupError != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusNotFound,
			"arquivo não encontrado",
		)
		return
	}

	analysis, originalManifest, manifestReadError :=
		loadExamManifest(
			request.Context(),
			client,
			bucket,
			payload.CaseID,
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
				) == payload.Path {
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

							if source != payload.Path {
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
			httpresponse.WriteError(
				responseWriter,
				http.StatusInternalServerError,
				"não foi possível atualizar o manifesto",
			)
			return
		}

		if _, manifestWriteError := client.PutObject(
			request.Context(),
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
			httpresponse.WriteError(
				responseWriter,
				http.StatusBadGateway,
				"não foi possível atualizar o manifesto",
			)
			return
		}
	}

	_, deleteObjectError := client.DeleteObject(
		request.Context(),
		&s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(payload.Path),
		},
	)

	if deleteObjectError != nil {
		if manifestChanged &&
			len(originalManifest) > 0 {
			_, _ = client.PutObject(
				request.Context(),
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

		httpresponse.WriteError(
			responseWriter,
			http.StatusBadGateway,
			"não foi possível excluir o arquivo",
		)
		return
	}

	if deleteMetadataError := handler.deleteExamRecord(payload.Path, bucket); deleteMetadataError != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusInternalServerError,
			"arquivo excluído, mas não foi possível excluir seus metadados",
		)
		return
	}

	responseWriter.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(responseWriter).Encode(
		map[string]any{
			"deleted":         true,
			"path":            payload.Path,
			"manifestUpdated": manifestChanged,
		},
	)
}
