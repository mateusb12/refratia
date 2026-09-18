package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	patientfeature "refratia/backend/features/patient"
	progressutil "refratia/backend/shared/progress"
)

const fileCheckpointVersion = "v1"

type fileCheckpointProcessingRun struct {
	StorageClient *s3.Client
	BucketName    string
	Force         bool
	StorageError  error
}

type fileCheckpointProcessingRunKey struct{}

type currentCheckpointFileKey struct{}

type processedFileCheckpoint struct {
	ProcessorVersion string         `json:"processorVersion"`
	SHA256           string         `json:"sha256"`
	Filename         string         `json:"filename"`
	ContentType      string         `json:"contentType"`
	Size             int64          `json:"size"`
	ProcessedAt      time.Time      `json:"processedAt"`
	Analysis         map[string]any `json:"analysis"`
}

type fileCheckpointLookupRequest struct {
	Files []struct {
		Filename string `json:"filename"`
		SHA256   string `json:"sha256"`
	} `json:"files"`
}

type fileCheckpointLookupResult struct {
	Filename    string `json:"filename"`
	SHA256      string `json:"sha256"`
	Status      string `json:"status"`
	ExamType    string `json:"examType,omitempty"`
	Eye         string `json:"eye,omitempty"`
	ProcessedAt string `json:"processedAt,omitempty"`
}

func withFileCheckpointStorage(
	processingContext context.Context,
	storageClientInstance *s3.Client,
	bucketName string,
	forceReprocess bool,
) context.Context {
	return context.WithValue(
		processingContext,
		fileCheckpointProcessingRunKey{},
		&fileCheckpointProcessingRun{
			StorageClient: storageClientInstance,
			BucketName:    bucketName,
			Force:         forceReprocess,
		},
	)
}

func withCurrentCheckpointFile(
	processingContext context.Context,
	file uploadedFile,
) context.Context {
	return context.WithValue(
		processingContext,
		currentCheckpointFileKey{},
		file,
	)
}

func fileCheckpointRunFromContext(
	processingContext context.Context,
) *fileCheckpointProcessingRun {
	checkpointRun, _ :=
		processingContext.Value(
			fileCheckpointProcessingRunKey{},
		).(*fileCheckpointProcessingRun)

	return checkpointRun
}

func currentCheckpointFileFromContext(
	processingContext context.Context,
) (uploadedFile, bool) {
	file, found :=
		processingContext.Value(
			currentCheckpointFileKey{},
		).(uploadedFile)

	return file, found
}

func rememberFileCheckpointStorageError(
	processingContext context.Context,
	storageError error,
) {
	checkpointRun :=
		fileCheckpointRunFromContext(
			processingContext,
		)

	if checkpointRun == nil {
		return
	}

	if storageError == nil {
		return
	}

	if checkpointRun.StorageError != nil {
		return
	}

	checkpointRun.StorageError =
		storageError
}

func currentFileCheckpointStorageError(
	processingContext context.Context,
) error {
	checkpointRun :=
		fileCheckpointRunFromContext(
			processingContext,
		)

	if checkpointRun == nil {
		return nil
	}

	return checkpointRun.StorageError
}

func validFileCheckpointSHA256(
	value string,
) bool {
	if len(value) != 64 {
		return false
	}

	decodedValue, decodeError :=
		hex.DecodeString(value)

	return decodeError == nil &&
		len(decodedValue) == 32
}

func uploadedFileCheckpointObjectKey(
	sha256Value string,
) string {
	return fmt.Sprintf(
		"file-checkpoints/%s/%s/source",
		fileCheckpointVersion,
		strings.ToLower(sha256Value),
	)
}

func processedFileCheckpointObjectKey(
	sha256Value string,
) string {
	return fmt.Sprintf(
		"file-checkpoints/%s/%s/result.json",
		fileCheckpointVersion,
		strings.ToLower(sha256Value),
	)
}

func storageObjectDoesNotExist(
	storageError error,
) bool {
	var storageAPIError smithy.APIError

	if !errors.As(
		storageError,
		&storageAPIError,
	) {
		return false
	}

	switch storageAPIError.ErrorCode() {
	case "NotFound", "NoSuchKey", "404":
		return true
	default:
		return false
	}
}

func uploadedFileCheckpointExists(
	processingContext context.Context,
	storageClientInstance *s3.Client,
	bucketName string,
	sha256Value string,
) (bool, error) {
	_, lookupError :=
		storageClientInstance.HeadObject(
			processingContext,
			&s3.HeadObjectInput{
				Bucket: aws.String(bucketName),
				Key: aws.String(
					uploadedFileCheckpointObjectKey(
						sha256Value,
					),
				),
			},
		)

	if lookupError == nil {
		return true, nil
	}

	if storageObjectDoesNotExist(
		lookupError,
	) {
		return false, nil
	}

	return false, lookupError
}

func saveUploadedFileBeforeProcessing(
	processingContext context.Context,
	storageClientInstance *s3.Client,
	bucketName string,
	file uploadedFile,
) error {
	fileAlreadySaved, lookupError :=
		uploadedFileCheckpointExists(
			processingContext,
			storageClientInstance,
			bucketName,
			file.Metadata.SHA256,
		)

	if lookupError != nil {
		return fmt.Errorf(
			"não foi possível verificar se %s já estava salvo: %w",
			file.Metadata.Filename,
			lookupError,
		)
	}

	if fileAlreadySaved {
		return nil
	}

	_, uploadError :=
		storageClientInstance.PutObject(
			processingContext,
			&s3.PutObjectInput{
				Bucket: aws.String(bucketName),
				Key: aws.String(
					uploadedFileCheckpointObjectKey(
						file.Metadata.SHA256,
					),
				),
				Body: bytes.NewReader(
					file.Data,
				),
				ContentType: aws.String(
					file.Metadata.ContentType,
				),
			},
		)

	if uploadError != nil {
		return fmt.Errorf(
			"não foi possível salvar %s antes do processamento: %w",
			file.Metadata.Filename,
			uploadError,
		)
	}

	return nil
}

func loadProcessedFileCheckpoint(
	processingContext context.Context,
	storageClientInstance *s3.Client,
	bucketName string,
	sha256Value string,
) (
	processedFileCheckpoint,
	bool,
	error,
) {
	checkpointObject, downloadError :=
		storageClientInstance.GetObject(
			processingContext,
			&s3.GetObjectInput{
				Bucket: aws.String(bucketName),
				Key: aws.String(
					processedFileCheckpointObjectKey(
						sha256Value,
					),
				),
			},
		)

	if downloadError != nil {
		if storageObjectDoesNotExist(
			downloadError,
		) {
			return processedFileCheckpoint{},
				false,
				nil
		}

		return processedFileCheckpoint{},
			false,
			downloadError
	}

	defer checkpointObject.Body.Close()

	var checkpoint processedFileCheckpoint

	decodeError :=
		json.NewDecoder(
			io.LimitReader(
				checkpointObject.Body,
				maxAnalysisSize+(1<<20),
			),
		).Decode(
			&checkpoint,
		)

	if decodeError != nil {
		return processedFileCheckpoint{},
			false,
			fmt.Errorf(
				"checkpoint inválido: %w",
				decodeError,
			)
	}

	if checkpoint.ProcessorVersion !=
		fileCheckpointVersion {
		return processedFileCheckpoint{},
			false,
			nil
	}

	if checkpoint.SHA256 !=
		sha256Value {
		return processedFileCheckpoint{},
			false,
			fmt.Errorf(
				"checkpoint possui SHA-256 divergente",
			)
	}

	if checkpoint.Analysis == nil {
		return processedFileCheckpoint{},
			false,
			fmt.Errorf(
				"checkpoint não possui análise",
			)
	}

	return checkpoint,
		true,
		nil
}

func saveProcessedFileCheckpoint(
	processingContext context.Context,
	storageClientInstance *s3.Client,
	bucketName string,
	checkpoint processedFileCheckpoint,
) error {
	encodedCheckpoint, encodeError :=
		json.MarshalIndent(
			checkpoint,
			"",
			"  ",
		)

	if encodeError != nil {
		return fmt.Errorf(
			"não foi possível serializar checkpoint: %w",
			encodeError,
		)
	}

	_, uploadError :=
		storageClientInstance.PutObject(
			processingContext,
			&s3.PutObjectInput{
				Bucket: aws.String(bucketName),
				Key: aws.String(
					processedFileCheckpointObjectKey(
						checkpoint.SHA256,
					),
				),
				Body: bytes.NewReader(
					encodedCheckpoint,
				),
				ContentType: aws.String(
					"application/json",
				),
			},
		)

	if uploadError != nil {
		return fmt.Errorf(
			"não foi possível persistir o resultado de %s: %w",
			checkpoint.Filename,
			uploadError,
		)
	}

	return nil
}

func cloneCheckpointMap(
	source map[string]any,
) map[string]any {
	if source == nil {
		return nil
	}

	encodedSource, encodeError :=
		json.Marshal(source)

	if encodeError != nil {
		return source
	}

	var clonedSource map[string]any

	decodeError :=
		json.Unmarshal(
			encodedSource,
			&clonedSource,
		)

	if decodeError != nil {
		return source
	}

	return clonedSource
}

func cloneCheckpointValue(
	source any,
) any {
	encodedSource, encodeError :=
		json.Marshal(source)

	if encodeError != nil {
		return source
	}

	var clonedSource any

	decodeError :=
		json.Unmarshal(
			encodedSource,
			&clonedSource,
		)

	if decodeError != nil {
		return source
	}

	return clonedSource
}

func checkpointExamKey(
	examType string,
) string {
	switch strings.ToUpper(
		strings.TrimSpace(examType),
	) {
	case "PENTACAM":
		return "pentacam_corneal_tomography"
	case "EYESUITE":
		return "iol_calculation"
	case "MICROSCOPIA_ESPECULAR":
		return "specular_microscopy"
	case "RETINA":
		return "fundus_retinography"
	default:
		return ""
	}
}

func identityEntriesForCheckpointFile(
	analysis map[string]any,
	filename string,
) []any {
	entries, _ :=
		analysis["verificacao_identidade"].([]any)

	result :=
		make(
			[]any,
			0,
		)

	for _, rawEntry := range entries {
		entry, _ :=
			rawEntry.(map[string]any)

		source, _ :=
			entry["source"].(string)

		if filepath.Base(source) !=
			filename {
			continue
		}

		result = append(
			result,
			cloneCheckpointValue(entry),
		)
	}

	return result
}

func buildAnalysisForFileCheckpoint(
	analysis map[string]any,
	file uploadedFile,
	examType string,
	eye string,
) map[string]any {
	examKey :=
		checkpointExamKey(
			examType,
		)

	if examKey == "" {
		return nil
	}

	exams, _ :=
		analysis["exams"].(map[string]any)

	rawExam, _ :=
		exams[examKey].(map[string]any)

	if rawExam == nil {
		return nil
	}

	checkpointAnalysis :=
		map[string]any{
			"exams":        map[string]any{},
			"source_files": []any{},
		}

	for _, metadataKey := range []string{
		"schema_version",
		"generated_on",
		"language",
		"conventions",
	} {
		if metadataValue, found :=
			analysis[metadataKey]; found {
			checkpointAnalysis[metadataKey] =
				cloneCheckpointValue(
					metadataValue,
				)
		}
	}

	checkpointExam :=
		cloneCheckpointMap(
			rawExam,
		)

	if eye == "OD" ||
		eye == "OS" {
		eyes, _ :=
			checkpointExam["eyes"].(map[string]any)

		if currentEye, found :=
			eyes[eye]; found {
			checkpointExam["eyes"] =
				map[string]any{
					eye: cloneCheckpointValue(
						currentEye,
					),
				}
		}
	}

	checkpointExam["source"] =
		[]any{
			file.Metadata.Filename,
		}

	checkpointExams :=
		checkpointAnalysis["exams"].(map[string]any)

	checkpointExams[examKey] =
		checkpointExam

	sourceMetadata :=
		map[string]any{
			"path":       file.Metadata.Filename,
			"type":       file.Metadata.ContentType,
			"size_bytes": file.Metadata.Size,
			"sha256":     file.Metadata.SHA256,
			"exam":       examKey,
		}

	if eye != "" {
		sourceMetadata["eye"] =
			eye
	}

	if existingSources, found :=
		analysis["source_files"].([]any); found {
		for _, rawSource := range existingSources {
			existingSource, _ :=
				rawSource.(map[string]any)

			if existingSource == nil {
				continue
			}

			path, _ :=
				existingSource["path"].(string)

			filename, _ :=
				existingSource["filename"].(string)

			if filepath.Base(path) !=
				file.Metadata.Filename &&
				filename !=
					file.Metadata.Filename {
				continue
			}

			sourceMetadata =
				cloneCheckpointMap(
					existingSource,
				)

			sourceMetadata["path"] =
				file.Metadata.Filename

			sourceMetadata["type"] =
				file.Metadata.ContentType

			sourceMetadata["size_bytes"] =
				file.Metadata.Size

			sourceMetadata["sha256"] =
				file.Metadata.SHA256

			sourceMetadata["exam"] =
				examKey

			if eye != "" {
				sourceMetadata["eye"] =
					eye
			}

			break
		}
	}

	checkpointAnalysis["source_files"] =
		[]any{
			sourceMetadata,
		}

	identityEntries :=
		identityEntriesForCheckpointFile(
			analysis,
			file.Metadata.Filename,
		)

	if len(identityEntries) > 0 {
		checkpointAnalysis["verificacao_identidade"] =
			identityEntries

		if patient, found :=
			analysis["patient"].(map[string]any); found {
			checkpointAnalysis["patient"] =
				cloneCheckpointMap(
					patient,
				)
		}
	}

	return checkpointAnalysis
}

func retinaCheckpointIsComplete(
	analysis map[string]any,
	eye string,
) bool {
	if eye != "OD" &&
		eye != "OS" {
		return false
	}

	exams, _ :=
		analysis["exams"].(map[string]any)

	exam, _ :=
		exams["fundus_retinography"].(map[string]any)

	if exam == nil {
		return false
	}

	examID, _ :=
		exam["id"].(string)

	if strings.TrimSpace(
		examID,
	) == "" {
		return false
	}

	deviceOrMode, _ :=
		exam["device_or_mode"].(string)

	if !strings.EqualFold(
		strings.TrimSpace(deviceOrMode),
		"Retina",
	) {
		return false
	}

	eyes, _ :=
		exam["eyes"].(map[string]any)

	eyePayload, _ :=
		eyes[eye].(map[string]any)

	if eyePayload == nil {
		return false
	}

	patientID, _ :=
		eyePayload["patient_id"].(string)

	if !strings.EqualFold(
		strings.TrimSpace(patientID),
		strings.TrimSpace(examID),
	) {
		return false
	}

	payloadEye, _ :=
		eyePayload["eye"].(string)

	if !strings.EqualFold(
		strings.TrimSpace(payloadEye),
		eye,
	) {
		return false
	}

	examDateTime, _ :=
		eyePayload["exam_datetime"].(string)

	return strings.TrimSpace(
		examDateTime,
	) != ""
}

func fileCheckpointResultIsComplete(
	analysis map[string]any,
	examType string,
	eye string,
) bool {
	switch strings.ToUpper(
		strings.TrimSpace(examType),
	) {
	case "PENTACAM":
		exams, _ :=
			analysis["exams"].(map[string]any)

		exam, _ :=
			exams["pentacam_corneal_tomography"].(map[string]any)

		if exam == nil {
			return false
		}

		eyes, _ :=
			exam["eyes"].(map[string]any)

		if eye == "OD" ||
			eye == "OS" {
			currentEye, _ :=
				eyes[eye].(map[string]any)

			if currentEye == nil {
				return false
			}

			return len(
				pentacamEyeLocalGaps(
					eye,
					currentEye,
				),
			) == 0
		}

		return pentacamLocalComplete(
			analysis,
		)

	case "EYESUITE":
		exams, _ :=
			analysis["exams"].(map[string]any)

		exam, _ :=
			exams["iol_calculation"].(map[string]any)

		if exam == nil {
			return false
		}

		return !iolNeedsRepair(exam)

	case "MICROSCOPIA_ESPECULAR":
		if eye == "AO" {
			return specularMicroscopyLocalComplete(
				analysis,
			)
		}

		exams, _ :=
			analysis["exams"].(map[string]any)

		exam, _ :=
			exams["specular_microscopy"].(map[string]any)

		if exam == nil {
			return false
		}

		eyes, _ :=
			exam["eyes"].(map[string]any)

		currentEye, _ :=
			eyes[eye].(map[string]any)

		if currentEye == nil {
			return false
		}

		cellDensity, found :=
			currentEye["cell_density_cells_per_mm2"].(float64)

		return found &&
			cellDensity > 0

	case "RETINA":
		return retinaCheckpointIsComplete(
			analysis,
			eye,
		)

	default:
		return false
	}
}

func mergeFileCheckpointAnalysis(
	currentAnalysis map[string]any,
	checkpointAnalysis map[string]any,
) (map[string]any, error) {
	currentIdentity,
		currentIdentifiable :=
		patientfeature.Identity(
			currentAnalysis,
		)

	checkpointIdentity,
		checkpointIdentifiable :=
		patientfeature.Identity(
			checkpointAnalysis,
		)

	if currentIdentifiable &&
		checkpointIdentifiable &&
		currentIdentity !=
			checkpointIdentity {
		return nil,
			fmt.Errorf(
				"arquivos pertencem a pacientes diferentes",
			)
	}

	currentVerification, _ :=
		currentAnalysis["verificacao_identidade"].([]any)

	checkpointVerification, _ :=
		checkpointAnalysis["verificacao_identidade"].([]any)

	mergedAnalysis :=
		patientfeature.Merge(
			currentAnalysis,
			checkpointAnalysis,
		)

	if len(currentVerification) > 0 ||
		len(checkpointVerification) > 0 {
		combinedVerification :=
			make(
				[]any,
				0,
				len(currentVerification)+
					len(checkpointVerification),
			)

		currentVerificationCopy, _ :=
			cloneCheckpointValue(
				currentVerification,
			).([]any)

		checkpointVerificationCopy, _ :=
			cloneCheckpointValue(
				checkpointVerification,
			).([]any)

		combinedVerification =
			append(
				combinedVerification,
				currentVerificationCopy...,
			)

		combinedVerification =
			append(
				combinedVerification,
				checkpointVerificationCopy...,
			)

		mergedAnalysis["verificacao_identidade"] =
			combinedVerification
	}

	return mergedAnalysis,
		nil
}

func replaceAnalysisContents(
	targetAnalysis map[string]any,
	sourceAnalysis map[string]any,
) {
	clear(targetAnalysis)

	for key, value := range sourceAnalysis {
		targetAnalysis[key] =
			value
	}
}

func renameCheckpointFilenameReferences(
	value any,
	previousFilename string,
	currentFilename string,
) {
	switch typedValue :=
		value.(type) {
	case map[string]any:
		for key, childValue := range typedValue {
			if textValue, isText :=
				childValue.(string); isText {
				if textValue ==
					previousFilename ||
					filepath.Base(
						textValue,
					) ==
						previousFilename {
					typedValue[key] =
						currentFilename
				}

				continue
			}

			renameCheckpointFilenameReferences(
				childValue,
				previousFilename,
				currentFilename,
			)
		}

	case []any:
		for index, childValue := range typedValue {
			if textValue, isText :=
				childValue.(string); isText {
				if textValue ==
					previousFilename ||
					filepath.Base(
						textValue,
					) ==
						previousFilename {
					typedValue[index] =
						currentFilename
				}

				continue
			}

			renameCheckpointFilenameReferences(
				childValue,
				previousFilename,
				currentFilename,
			)
		}
	}
}

func refreshCheckpointSourceMetadata(
	analysis map[string]any,
	file uploadedFile,
) {
	sourceFiles, _ :=
		analysis["source_files"].([]any)

	for _, rawSource := range sourceFiles {
		sourceMetadata, _ :=
			rawSource.(map[string]any)

		if sourceMetadata == nil {
			continue
		}

		sourceMetadata["path"] =
			file.Metadata.Filename

		sourceMetadata["type"] =
			file.Metadata.ContentType

		sourceMetadata["size_bytes"] =
			file.Metadata.Size

		sourceMetadata["sha256"] =
			file.Metadata.SHA256
	}
}

func emitReusedFileCheckpoint(
	processingContext context.Context,
	file uploadedFile,
	checkpointAnalysis map[string]any,
) {
	examType :=
		localExamTypeFromFilename(
			file.Metadata.Filename,
		)

	eye :=
		localExamEyeFromFilename(
			file.Metadata.Filename,
		)

	progressutil.Report(
		processingContext,
		100,
		"checkpoint",
		"Resultado anterior reutilizado",
	)

	emitLocalPartial(
		processingContext,
		file.Metadata.Filename,
		"Resultado anterior reutilizado",
		checkpointAnalysis,
	)

	progressutil.Emit(
		processingContext,
		progressutil.Event{
			Type:     "file_result",
			Percent:  100,
			Stage:    "file_result",
			Message:  "Resultado reutilizado; OCR não executado",
			Filename: file.Metadata.Filename,
			Payload: map[string]any{
				"filename": file.Metadata.Filename,
				"examType": examType,
				"eye":      eye,
				"status":   "extracted",
				"message":  "Resultado reutilizado; OCR não executado",
				"analysis": checkpointAnalysis,
				"reused":   true,
			},
		},
	)
}

func prepareFileForIdempotentProcessing(
	processingContext context.Context,
	file uploadedFile,
	analysis map[string]any,
) (bool, error) {
	checkpointRun :=
		fileCheckpointRunFromContext(
			processingContext,
		)

	if checkpointRun == nil {
		return false,
			nil
	}

	if !checkpointRun.Force {
		checkpoint,
			found,
			loadError :=
			loadProcessedFileCheckpoint(
				processingContext,
				checkpointRun.StorageClient,
				checkpointRun.BucketName,
				file.Metadata.SHA256,
			)

		if loadError != nil {
			return false,
				fmt.Errorf(
					"não foi possível consultar resultado anterior de %s: %w",
					file.Metadata.Filename,
					loadError,
				)
		}

		if found {
			reusedAnalysis :=
				cloneCheckpointMap(
					checkpoint.Analysis,
				)

			if checkpoint.Filename !=
				file.Metadata.Filename {
				renameCheckpointFilenameReferences(
					reusedAnalysis,
					checkpoint.Filename,
					file.Metadata.Filename,
				)
			}

			refreshCheckpointSourceMetadata(
				reusedAnalysis,
				file,
			)

			mergedAnalysis,
				mergeError :=
				mergeFileCheckpointAnalysis(
					analysis,
					reusedAnalysis,
				)

			if mergeError != nil {
				return false,
					mergeError
			}

			replaceAnalysisContents(
				analysis,
				mergedAnalysis,
			)

			emitReusedFileCheckpoint(
				processingContext,
				file,
				analysis,
			)

			return true,
				nil
		}
	}

	saveError :=
		saveUploadedFileBeforeProcessing(
			processingContext,
			checkpointRun.StorageClient,
			checkpointRun.BucketName,
			file,
		)

	if saveError != nil {
		return false,
			saveError
	}

	return false,
		nil
}

func saveCompletedFileCheckpointFromContext(
	processingContext context.Context,
	filename string,
	examType string,
	eye string,
	status string,
	analysis map[string]any,
) error {
	if status != "extracted" {
		return nil
	}

	if analysis == nil {
		return nil
	}

	checkpointRun :=
		fileCheckpointRunFromContext(
			processingContext,
		)

	if checkpointRun == nil {
		return nil
	}

	file, found :=
		currentCheckpointFileFromContext(
			processingContext,
		)

	if !found {
		return nil
	}

	if file.Metadata.Filename !=
		filename {
		return nil
	}

	if !fileCheckpointResultIsComplete(
		analysis,
		examType,
		eye,
	) {
		return nil
	}

	checkpointAnalysis :=
		buildAnalysisForFileCheckpoint(
			analysis,
			file,
			examType,
			eye,
		)

	if checkpointAnalysis == nil {
		return nil
	}

	return saveProcessedFileCheckpoint(
		processingContext,
		checkpointRun.StorageClient,
		checkpointRun.BucketName,
		processedFileCheckpoint{
			ProcessorVersion: fileCheckpointVersion,
			SHA256:           file.Metadata.SHA256,
			Filename:         file.Metadata.Filename,
			ContentType:      file.Metadata.ContentType,
			Size:             file.Metadata.Size,
			ProcessedAt:      time.Now().UTC(),
			Analysis:         checkpointAnalysis,
		},
	)
}

func checkFileCheckpoints(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	if request.Method !=
		http.MethodPost {
		responseWriter.WriteHeader(
			http.StatusMethodNotAllowed,
		)
		return
	}

	request.Body =
		http.MaxBytesReader(
			responseWriter,
			request.Body,
			256<<10,
		)

	var lookupRequest fileCheckpointLookupRequest

	decodeError :=
		json.NewDecoder(
			request.Body,
		).Decode(
			&lookupRequest,
		)

	if decodeError != nil {
		writeError(
			responseWriter,
			http.StatusBadRequest,
			"consulta de arquivos inválida",
		)
		return
	}

	if len(lookupRequest.Files) == 0 ||
		len(lookupRequest.Files) > 100 {
		writeError(
			responseWriter,
			http.StatusBadRequest,
			"envie entre 1 e 100 arquivos",
		)
		return
	}

	storageClientInstance,
		storageClientError :=
		storageClient(
			request.Context(),
		)

	if storageClientError != nil {
		writeError(
			responseWriter,
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	bucketName :=
		os.Getenv(
			"BUCKET_NAME",
		)

	results :=
		make(
			[]fileCheckpointLookupResult,
			0,
			len(lookupRequest.Files),
		)

	for _, file := range lookupRequest.Files {
		if !validFileCheckpointSHA256(
			file.SHA256,
		) {
			writeError(
				responseWriter,
				http.StatusBadRequest,
				"SHA-256 inválido",
			)
			return
		}

		result :=
			fileCheckpointLookupResult{
				Filename: file.Filename,
				SHA256:   file.SHA256,
				ExamType: localExamTypeFromFilename(
					file.Filename,
				),
				Eye: localExamEyeFromFilename(
					file.Filename,
				),
			}

		checkpoint,
			processed,
			checkpointLoadError :=
			loadProcessedFileCheckpoint(
				request.Context(),
				storageClientInstance,
				bucketName,
				file.SHA256,
			)

		if checkpointLoadError != nil {
			writeError(
				responseWriter,
				http.StatusBadGateway,
				"não foi possível consultar os resultados salvos",
			)
			return
		}

		if processed {
			result.Status =
				"processed"

			result.ProcessedAt =
				checkpoint.ProcessedAt.
					Format(
						time.RFC3339,
					)

			results = append(
				results,
				result,
			)

			continue
		}

		sourceSaved,
			sourceLookupError :=
			uploadedFileCheckpointExists(
				request.Context(),
				storageClientInstance,
				bucketName,
				file.SHA256,
			)

		if sourceLookupError != nil {
			writeError(
				responseWriter,
				http.StatusBadGateway,
				"não foi possível consultar os arquivos salvos",
			)
			return
		}

		if sourceSaved {
			result.Status = "saved"
		} else {
			result.Status = "new"
		}

		results = append(
			results,
			result,
		)
	}

	responseWriter.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(
		responseWriter,
	).Encode(
		map[string]any{
			"processorVersion": fileCheckpointVersion,
			"files":            results,
		},
	)
}
