package intake

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
	"refratia/backend/src/shared/profiling"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"refratia/backend/src/features/fileIdentification/classification"
	patientfeature "refratia/backend/src/features/persistence/patient"
	"refratia/backend/src/shared/fileStorage"
	"refratia/backend/src/shared/httpresponse"
	progress "refratia/backend/src/shared/progress"
)

// fileCheckpointVersion keys every checkpoint object. Bump it whenever an
// extractor's output changes: older checkpoints then miss and the document is
// extracted again. v2: v1 results were never written (completeness checked
// legacy labels), and v2 records the classification inputs. v3: the result
// holds only the file's own identity reading, never the batch's patient. v4:
// whole-exam writers (IOL, specular) keep the exam as written and replace it
// on a hit. v5: Pentacam identity dates are read in full or not at all, and the
// Z31 axis no longer reads the duplicate of the magnitude cell. v6: Pentacam
// eyes carry the Belin ABCD staging line. v7: staging grades keep their "+". v8: Pentacam
// birth dates are checked for plausibility after padding, and a malformed exam
// date sends the identity reader to the next screen. v9: Pentacam ISV cells with
// the border glued to the leading digit are read from the cell interior, KI is
// found on colour-filled cells, and EyeSuite K axes survive a stray tick
// before "@".
const fileCheckpointVersion = "v9"

type fileCheckpointProcessingRun struct {
	StorageClient fileStorage.Repository
	BucketName    string
	Force         bool
	// Quando informado, os SHA-256 presentes aqui serão processados novamente.
	// Nil preserva o comportamento padrão de reutilizar checkpoints válidos.
	ProcessFiles map[string]bool
	StorageError error
}

type fileCheckpointProcessingRunKey struct{}

type currentCheckpointFileKey struct{}

// extractionStartKey carries the analysis as it was before the current file's
// extraction, so its checkpoint holds only what that extraction wrote.
type extractionStartKey struct{}

type extractionStart struct {
	eyes            map[string]string // snapshotEyes
	identityEntries int               // len(verificacao_identidade)
}

func withExtractionStart(processingContext context.Context, analysis map[string]any) context.Context {
	entries, _ := analysis["verificacao_identidade"].([]any)
	return context.WithValue(processingContext, extractionStartKey{}, extractionStart{snapshotEyes(analysis), len(entries)})
}

// errCheckpointConflict: the checkpoint disagrees with the batch's identity;
// the file is extracted instead, as if it had no checkpoint.
var errCheckpointConflict = errors.New("checkpoint identity conflict")

type processedFileCheckpoint struct {
	ProcessorVersion string    `json:"processorVersion"`
	SHA256           string    `json:"sha256"`
	Filename         string    `json:"filename"`
	ContentType      string    `json:"contentType"`
	Size             int64     `json:"size"`
	ProcessedAt      time.Time `json:"processedAt"`
	// Contract exam key and eye the result was extracted and checked as.
	ExamType string `json:"examType"`
	Eye      string `json:"eye"`
	// The pre-OCR classification inputs (declared type/eye, else the
	// filename's). Content classification is a function of the bytes, so the
	// SHA-256 already covers it.
	HintExamType string         `json:"hintExamType"`
	HintEye      string         `json:"hintEye"`
	Analysis     map[string]any `json:"analysis"`
}

type fileCheckpointLookupRequest struct {
	Files []struct {
		Filename string `json:"filename"`
		SHA256   string `json:"sha256"`
	} `json:"files"`
}

type fileCheckpointLookupResult struct {
	Filename        string   `json:"filename"`
	SHA256          string   `json:"sha256"`
	Status          string   `json:"status"`
	ExamType        string   `json:"examType,omitempty"`
	Eye             string   `json:"eye,omitempty"`
	ProcessedAt     string   `json:"processedAt,omitempty"`
	ExtractedFields []string `json:"extractedFields,omitempty"`
}

func checkpointExtractedFields(
	analysis map[string]any,
	examType string,
) []string {
	exams, _ := analysis["exams"].(map[string]any)
	exam, _ := exams[checkpointExamKey(examType)].(map[string]any)
	if exam == nil {
		return nil
	}

	fields := make([]string, 0)
	var walk func(map[string]any, []string)
	walk = func(values map[string]any, path []string) {
		for key, value := range values {
			if key == "source" || value == nil {
				continue
			}

			childPath := append(append([]string{}, path...), key)
			switch child := value.(type) {
			case map[string]any:
				walk(child, childPath)
			case string:
				if strings.TrimSpace(child) != "" {
					fields = append(fields, strings.Join(childPath, "."))
				}
			case float64, float32, int, int64, json.Number, bool:
				fields = append(fields, strings.Join(childPath, "."))
			}
		}
	}

	walk(exam, nil)
	sort.Strings(fields)
	return fields
}

func withFileCheckpointStorage(
	processingContext context.Context,
	storageClientInstance fileStorage.Repository,
	bucketName string,
	forceReprocess bool,
	processFiles map[string]bool,
) context.Context {
	return context.WithValue(
		processingContext,
		fileCheckpointProcessingRunKey{},
		&fileCheckpointProcessingRun{
			StorageClient: storageClientInstance,
			BucketName:    bucketName,
			Force:         forceReprocess,
			ProcessFiles:  processFiles,
		},
	)
}

func withCurrentCheckpointFile(
	processingContext context.Context,
	file UploadedFile,
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
) (UploadedFile, bool) {
	file, found :=
		processingContext.Value(
			currentCheckpointFileKey{},
		).(UploadedFile)

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
		fileStorage.CheckpointsPrefix+"%s/%s/source",
		fileCheckpointVersion,
		strings.ToLower(sha256Value),
	)
}

func processedFileCheckpointObjectKey(
	sha256Value string,
) string {
	return fmt.Sprintf(
		fileStorage.CheckpointsPrefix+"%s/%s/result.json",
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
	storageClientInstance fileStorage.Repository,
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
	storageClientInstance fileStorage.Repository,
	bucketName string,
	file UploadedFile,
) error {
	defer profiling.Start(processingContext, "checkpoint.save_source").End(nil)
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

// loadProcessedFileCheckpoint returns the checkpoint, or the reason it can't
// be used ("not_found", "version", "unreadable"). Only a storage failure is an
// error: a corrupt or foreign checkpoint is a miss and the file is extracted.
func loadProcessedFileCheckpoint(
	processingContext context.Context,
	storageClientInstance fileStorage.Repository,
	bucketName string,
	sha256Value string,
) (
	processedFileCheckpoint,
	string,
	error,
) {
	defer profiling.Start(processingContext, "checkpoint.load_result").End(nil)
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
				"not_found",
				nil
		}

		return processedFileCheckpoint{},
			"",
			downloadError
	}

	defer checkpointObject.Body.Close()

	var checkpoint processedFileCheckpoint

	decodeError :=
		json.NewDecoder(
			io.LimitReader(
				checkpointObject.Body,
				fileStorage.MaxAnalysisSize+(1<<20),
			),
		).Decode(
			&checkpoint,
		)

	switch {
	case decodeError != nil, checkpoint.SHA256 != sha256Value, checkpoint.Analysis == nil:
		return processedFileCheckpoint{}, "unreadable", nil
	case checkpoint.ProcessorVersion != fileCheckpointVersion:
		return processedFileCheckpoint{}, "version", nil
	}

	return checkpoint,
		"",
		nil
}

func saveProcessedFileCheckpoint(
	processingContext context.Context,
	storageClientInstance fileStorage.Repository,
	bucketName string,
	checkpoint processedFileCheckpoint,
) error {
	defer profiling.Start(processingContext, "checkpoint.save_result").End(nil)
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

// wholeExamWriters are the exams whose writer replaces the whole exam, so the
// batch keeps only the last file of the type (tryExtractEyeSuiteLocal,
// tryExtractSpecularMicroscopyLocal). The others write one eye at a time.
var wholeExamWriters = map[string]bool{
	"iol_calculation":     true,
	"specular_microscopy": true,
}

func checkpointExamKey(
	examType string,
) string {
	key, _ := classification.ContractExamType(examType)
	return key
}

// identityEntriesForCheckpointFile returns the identity readings this file's
// extraction appended (entries past before). ok is false when an earlier file
// of the batch already left a reading under the same filename: the writers
// then skip theirs, and attribution by name is ambiguous.
func identityEntriesForCheckpointFile(
	analysis map[string]any,
	filename string,
	before int,
) (result []any, ok bool) {
	entries, _ :=
		analysis["verificacao_identidade"].([]any)

	for index, rawEntry := range entries {
		entry, _ :=
			rawEntry.(map[string]any)

		source, _ :=
			entry["source"].(string)

		if source != filename {
			continue
		}

		if index < before {
			return nil, false
		}

		result = append(
			result,
			cloneCheckpointValue(entry),
		)
	}

	return result, true
}

func buildAnalysisForFileCheckpoint(
	analysis map[string]any,
	file UploadedFile,
	examType string,
	eye string,
	eyesBefore map[string]string,
	identityBefore int,
) map[string]any {
	identityEntries, attributable :=
		identityEntriesForCheckpointFile(
			analysis,
			file.Metadata.Filename,
			identityBefore,
		)

	if !attributable {
		return nil
	}

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

	// Only what the extraction itself wrote for this file: source_files and
	// the envelope metadata are filled after extraction (enrichSourceFiles),
	// so a checkpoint carrying them would change Merge's re-upload grouping
	// and the analysis date.
	checkpointAnalysis :=
		map[string]any{
			"exams": map[string]any{},
		}

	checkpointExam :=
		cloneCheckpointMap(
			rawExam,
		)

	// A whole-exam writer leaves the exam holding this file alone, so it is
	// kept as written; it must be this file's (source is set to it alone).
	if wholeExamWriters[examKey] {
		if sources, _ := rawExam["source"].([]any); len(sources) != 1 || sources[0] != file.Metadata.Filename {
			return nil
		}
		// An eye equal to the previous file's got no field diagnostics here
		// (validateTouchedEyes); alone it would: not checkpointed.
		after := snapshotEyes(analysis)
		eyes, _ := checkpointExam["eyes"].(map[string]any)
		for eyeName := range eyes {
			if eyeKey := examKey + "/" + eyeName; eyesBefore[eyeKey] == after[eyeKey] {
				return nil
			}
		}
	} else if eyes, hasEyes := checkpointExam["eyes"].(map[string]any); hasEyes {
		// Keep only the eyes this file wrote (changed since before its
		// extraction), and only its declared eye: the shared exam also holds
		// what other files of the upload extracted.
		after := snapshotEyes(analysis)
		kept := map[string]any{}
		for eyeName, payload := range eyes {
			eyeKey := examKey + "/" + eyeName
			if eyesBefore[eyeKey] == after[eyeKey] {
				continue
			}
			if (eye == "OD" || eye == "OS") && eyeName != eye {
				continue
			}
			kept[eyeName] = payload
		}
		if len(kept) == 0 {
			return nil
		}
		checkpointExam["eyes"] = kept
	}

	checkpointExam["source"] =
		[]any{
			file.Metadata.Filename,
		}

	checkpointExams :=
		checkpointAnalysis["exams"].(map[string]any)

	checkpointExams[examKey] =
		checkpointExam

	// The file's field diagnostics travel with its result: the preview and the
	// run audit read them per file_sha256.
	diagnostics, _ := analysis["extraction_diagnostics"].([]any)
	fileDiagnostics := []any{}
	for _, rawDiagnostic := range diagnostics {
		if diagnostic, _ := rawDiagnostic.(map[string]any); diagnostic["file_sha256"] == file.Metadata.SHA256 {
			fileDiagnostics = append(fileDiagnostics, cloneCheckpointValue(diagnostic))
		}
	}
	if len(fileDiagnostics) > 0 {
		checkpointAnalysis["extraction_diagnostics"] = fileDiagnostics
	}

	if len(identityEntries) > 0 {
		checkpointAnalysis["verificacao_identidade"] =
			identityEntries
	}

	// The batch's patient is every file's evidence. A file contributes only
	// the name it read: the identity writers fill an empty full_name with it
	// (birth_date is derived from the readings).
	for _, rawEntry := range identityEntries {
		if name, found := rawEntry.(map[string]any)["nome_lido"]; found {
			checkpointAnalysis["patient"] = map[string]any{"full_name": name}
			break
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

// pentacamTKCPrintedDash reports whether the document fileSHA256 recorded, for
// eye, a TKC observed as not applicable: the printed dash (recognition stage;
// a region not_applicable means the screen was absent, not read).
func pentacamTKCPrintedDash(analysis map[string]any, fileSHA256, eye string) bool {
	diagnostics, _ := analysis["extraction_diagnostics"].([]any)
	for _, rawDiagnostic := range diagnostics {
		diagnostic, _ := rawDiagnostic.(map[string]any)
		if fileSHA256 != "" && diagnostic["file_sha256"] == fileSHA256 &&
			diagnostic["exam_type"] == "pentacam_corneal_tomography" && diagnostic["eye"] == eye &&
			diagnostic["field"] == "tkc" && diagnostic["status"] == "not_applicable" && diagnostic["stage"] == "recognition" {
			return true
		}
	}
	return false
}

// fileCheckpointResultIsComplete: fileSHA256 is the document the result
// belongs to (its diagnostics are matched by it).
func fileCheckpointResultIsComplete(
	analysis map[string]any,
	examType string,
	eye string,
	fileSHA256 string,
) bool {
	// Callers pass contract keys (pentacam_corneal_tomography, ...);
	// legacy labels (PENTACAM, ...) map onto the same keys.
	switch checkpointExamKey(examType) {
	case "pentacam_corneal_tomography":
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

			gaps := pentacamEyeLocalGaps(eye, currentEye)
			// An absent TKC is complete only as this document's printed dash.
			gaps = slices.DeleteFunc(gaps, func(gap string) bool {
				return gap == "pentacam."+eye+".topometric_indices_8mm.tkc" &&
					pentacamTKCPrintedDash(analysis, fileSHA256, eye)
			})
			return len(gaps) == 0
		}

		// Without the file's eye the result can't be told apart from the
		// other report's eye in the same exam: not checkpointed.
		return false

	case "iol_calculation":
		exams, _ :=
			analysis["exams"].(map[string]any)

		exam, _ :=
			exams["iol_calculation"].(map[string]any)

		if exam == nil {
			return false
		}

		return !iolNeedsRepair(exam)

	case "specular_microscopy":
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

	case "fundus_retinography":
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
	_,
		currentIdentifiable :=
		patientfeature.Identity(
			currentAnalysis,
		)

	_,
		checkpointIdentifiable :=
		patientfeature.Identity(
			checkpointAnalysis,
		)

	if currentIdentifiable &&
		checkpointIdentifiable &&
		!patientfeature.SameIdentity(
			currentAnalysis,
			checkpointAnalysis,
		) {
		return nil, errCheckpointConflict
	}

	// An exam id the batch already holds must match (the retina extractor
	// rejects a photo of another id).
	currentExams, _ := currentAnalysis["exams"].(map[string]any)
	checkpointExams, _ := checkpointAnalysis["exams"].(map[string]any)
	for examKey, rawExam := range checkpointExams {
		checkpointID, _ := rawExam.(map[string]any)["id"].(string)
		currentExam, _ := currentExams[examKey].(map[string]any)
		currentID, _ := currentExam["id"].(string)
		if strings.TrimSpace(currentID) != "" && strings.TrimSpace(checkpointID) != "" &&
			!strings.EqualFold(strings.TrimSpace(currentID), strings.TrimSpace(checkpointID)) {
			return nil, errCheckpointConflict
		}
	}

	// Merge takes only the per-eye exams; whole-exam writers replace theirs,
	// and the patient and the per-file lists are applied below the way the
	// extractors write them.
	perEyeExams := map[string]any{}
	for examKey, exam := range checkpointExams {
		if !wholeExamWriters[examKey] {
			perEyeExams[examKey] = exam
		}
	}
	mergedAnalysis :=
		patientfeature.Merge(
			currentAnalysis,
			map[string]any{"exams": perEyeExams},
		)

	// Merge always writes source_files; extraction leaves it to
	// enrichSourceFiles, so keep the fresh shape.
	if sources, _ := mergedAnalysis["source_files"].([]any); len(sources) == 0 {
		if _, had := currentAnalysis["source_files"]; !had {
			delete(mergedAnalysis, "source_files")
		}
	}

	// Extractors append their file to exam.source; Merge replaces it.
	mergedExams, _ := mergedAnalysis["exams"].(map[string]any)
	for examKey, exam := range checkpointExams {
		if wholeExamWriters[examKey] {
			mergedExams[examKey] = cloneCheckpointValue(exam)
		}
	}
	for examKey := range perEyeExams {
		currentExam, _ := currentExams[examKey].(map[string]any)
		mergedExam, _ := mergedExams[examKey].(map[string]any)
		if currentExam == nil || mergedExam == nil {
			continue
		}
		sources, _ := cloneCheckpointValue(currentExam["source"]).([]any)
		checkpointSources, _ := checkpointExams[examKey].(map[string]any)["source"].([]any)
		for _, source := range checkpointSources {
			if !slices.Contains(sources, source) {
				sources = append(sources, source)
			}
		}
		mergedExam["source"] = sources
	}

	// Per-file evidence lists accumulate across files; Merge would replace them.
	for _, listKey := range []string{"verificacao_identidade", "extraction_diagnostics"} {
		currentEntries, _ := currentAnalysis[listKey].([]any)
		checkpointEntries, _ := checkpointAnalysis[listKey].([]any)
		if len(currentEntries)+len(checkpointEntries) == 0 {
			continue
		}
		combined, _ := cloneCheckpointValue(currentEntries).([]any)
		checkpointCopy, _ := cloneCheckpointValue(checkpointEntries).([]any)
		mergedAnalysis[listKey] = append(combined, checkpointCopy...)
	}

	// As mergeLocalPatientIdentity: fill an empty name, then derive the
	// identity from all readings.
	if checkpointPatient, found := checkpointAnalysis["patient"].(map[string]any); found {
		patient, _ := mergedAnalysis["patient"].(map[string]any)
		if patient == nil {
			patient = map[string]any{}
			mergedAnalysis["patient"] = patient
		}
		if current, _ := patient["full_name"].(string); strings.TrimSpace(current) == "" {
			patient["full_name"] = checkpointPatient["full_name"]
		}
		patientfeature.NormalizeIdentityFields(mergedAnalysis)
		if _, resolved := patientfeature.CanonicalBirthDate(mergedAnalysis); !resolved {
			delete(patient, "birth_date")
		}
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
	file UploadedFile,
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
	file UploadedFile,
	examType string,
	eye string,
	checkpointAnalysis map[string]any,
) {

	progress.Report(
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

	progress.Emit(
		processingContext,
		progress.Event{
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

// prepareFileForIdempotentProcessing merges a reusable checkpoint into
// analysis and reports true, or reports why the file must be extracted
// ("forced", "selected", "not_found", "version", "unreadable",
// "classification_inputs", "incomplete", "identity_conflict").
func prepareFileForIdempotentProcessing(
	processingContext context.Context,
	file UploadedFile,
	analysis map[string]any,
) (bool, string, error) {
	checkpointRun :=
		fileCheckpointRunFromContext(
			processingContext,
		)

	if checkpointRun == nil {
		return false, "", nil
	}

	missReason := "forced"
	if !checkpointRun.Force {
		missReason = ""
		if checkpointRun.ProcessFiles[file.Metadata.SHA256] {
			missReason = "selected"
		}
	}

	if missReason == "" {
		checkpoint,
			loadMiss,
			loadError :=
			loadProcessedFileCheckpoint(
				processingContext,
				checkpointRun.StorageClient,
				checkpointRun.BucketName,
				file.Metadata.SHA256,
			)

		if loadError != nil {
			return false, "",
				fmt.Errorf(
					"não foi possível consultar resultado anterior de %s: %w",
					file.Metadata.Filename,
					loadError,
				)
		}

		hintExamType, hintEye := localClassificationHint(file)
		switch {
		case loadMiss != "":
			missReason = loadMiss
		case checkpoint.HintExamType != hintExamType || checkpoint.HintEye != hintEye:
			missReason = "classification_inputs"
		case !fileCheckpointResultIsComplete(checkpoint.Analysis, checkpoint.ExamType, checkpoint.Eye, checkpoint.SHA256):
			missReason = "incomplete"
		}

		var mergedAnalysis map[string]any
		if missReason == "" {
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

			var mergeError error
			mergedAnalysis, mergeError =
				mergeFileCheckpointAnalysis(
					analysis,
					reusedAnalysis,
				)

			switch {
			case errors.Is(mergeError, errCheckpointConflict):
				missReason = "identity_conflict"
			case mergeError != nil:
				return false, "",
					mergeError
			}
		}

		if missReason == "" {
			replaceAnalysisContents(
				analysis,
				mergedAnalysis,
			)

			emitReusedFileCheckpoint(
				processingContext,
				file,
				checkpoint.ExamType,
				checkpoint.Eye,
				analysis,
			)

			return true, "", nil
		}

		// Com uma seleção explícita, o toggle desligado também permite
		// deixar de fora um arquivo sem checkpoint anterior.
		if checkpointRun.ProcessFiles != nil {
			return true, "", nil
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
		return false, missReason,
			saveError
	}

	return false, missReason, nil
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
		file.Metadata.SHA256,
	) {
		return nil
	}

	start, _ := processingContext.Value(extractionStartKey{}).(extractionStart)
	checkpointAnalysis :=
		buildAnalysisForFileCheckpoint(
			analysis,
			file,
			examType,
			eye,
			start.eyes,
			start.identityEntries,
		)

	hintExamType, hintEye := localClassificationHint(file)

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
			ExamType:         examType,
			Eye:              eye,
			HintExamType:     hintExamType,
			HintEye:          hintEye,
			Analysis:         checkpointAnalysis,
		},
	)
}

func CheckFileCheckpoints(
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
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"consulta de arquivos inválida",
		)
		return
	}

	if len(lookupRequest.Files) == 0 ||
		len(lookupRequest.Files) > 100 {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"envie entre 1 e 100 arquivos",
		)
		return
	}

	storageClientInstance,
		storageClientError :=
		fileStorage.New(
			request.Context(),
		)

	if storageClientError != nil {
		httpresponse.WriteError(
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
			httpresponse.WriteError(
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
				ExamType: classification.ExamTypeFromFilename(
					file.Filename,
				),
				Eye: classification.EyeFromFilename(
					file.Filename,
				),
			}

		checkpoint,
			checkpointMiss,
			checkpointLoadError :=
			loadProcessedFileCheckpoint(
				request.Context(),
				storageClientInstance,
				bucketName,
				file.SHA256,
			)

		if checkpointLoadError != nil {
			httpresponse.WriteError(
				responseWriter,
				http.StatusBadGateway,
				"não foi possível consultar os resultados salvos",
			)
			return
		}

		if checkpointMiss == "" &&
			!fileCheckpointResultIsComplete(checkpoint.Analysis, checkpoint.ExamType, checkpoint.Eye, checkpoint.SHA256) {
			checkpointMiss = "incomplete" // analyze would extract it again
		}

		if checkpointMiss == "" {
			result.ExamType, result.Eye = checkpoint.ExamType, checkpoint.Eye
			result.ProcessedAt =
				checkpoint.ProcessedAt.
					Format(
						time.RFC3339,
					)
			result.ExtractedFields = checkpointExtractedFields(
				checkpoint.Analysis,
				result.ExamType,
			)

			if len(result.ExtractedFields) > 0 {
				result.Status = "processed"
			} else {
				// Um checkpoint sem campos não é um resultado concluído.
				result.Status = "saved"
			}

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
			httpresponse.WriteError(
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
