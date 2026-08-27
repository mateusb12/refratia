package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const extractionPrompt = `Extraia e consolide TODOS os dados alfanuméricos legíveis dos documentos oftalmológicos enviados em um único JSON. Não faça diagnóstico, não invente valores e use null quando um dado não estiver visível.

Organize o resultado como paciente_compilado.json:
- schema_version, generated_on (AAAA-MM-DD) e language (pt-BR);
- patient: full_name, birth_date normalizada e formatos conflitantes encontrados;
- facility: nome, descrição, endereço e telefone quando existirem;
- conventions: OD, OS, AO, significado de null e separador decimal normalizado;
- source_files: um item por arquivo com path igual ao nome recebido, exam, eye, páginas/dimensões e conteúdo por página quando identificável;
- exams: use exatamente as chaves fundus_retinography, iol_calculation, pentacam_corneal_tomography e specular_microscopy quando aplicáveis. Inclua SOMENTE exames realmente evidenciados pelos arquivos; não crie chaves para exames ausentes. Cada exame deve ter source com os nomes dos arquivos correspondentes. Preserve aparelho, software, data/hora, qualidade, alertas, fonte e TODOS os campos, índices, medições, eixos, tabelas e cálculos legíveis. Separe olhos em eyes.OD e eyes.OS (ou AO quando realmente conjunto). Use nomes de campos técnicos em snake_case e inclua unidades no nome quando isso remover ambiguidade; não substitua a hierarquia específica do equipamento por um modelo genérico;
- extraction_notes: method, scope, not_encoded e clinical_use_warning.

Contrato mínimo de campos por exame (não invente valores; quando não estiver no arquivo, registre como ausente):
- pentacam_corneal_tomography: K1, K2, Km, astigmatismo corneano anterior, paquimetria do ponto mais fino, BAD-D, ARTmax, ISV, IVA, IHA, KI, CKI, TKC, coma Z31 zona 5 mm, ACD e Z40 zona 6 mm;
- iol_calculation: comprimento axial, K1, K2, Km, astigmatismo e eixo da biometria, ACD, espessura do cristalino, white-to-white e refração alvo;
- specular_microscopy: contagem/densidade endotelial;
- fundus_retinography: ID do paciente, data/hora e achados/observações da imagem.
Não inclua um exame no objeto "exams" apenas porque ele é esperado pelo protocolo: inclua somente exames evidenciados pelos arquivos enviados.

Confronte identidade, datas e lateralidade entre os arquivos. Preserve avisos do equipamento e divergências do documento. Não resuma tabelas nem omita linhas repetidas por modelo de lente. Retorne somente um objeto JSON.`

const pentacamRepairPrompt = `Analise somente os PDFs do Pentacam enviados e extraia os campos abaixo. Cada PDF pode ter várias páginas: examine todas, especialmente a página "Ectasia Reforçada Belin / Ambrósio".

Retorne somente este objeto JSON, mantendo null apenas quando o valor realmente não estiver legível em nenhuma página:
{"eyes":{"OD":{"general":{"pachymetry_thinnest_um":null,"k_max_anterior_diopters":null},"belin_ambrosio":{"d":null,"art_max":null}},"OS":{"general":{"pachymetry_thinnest_um":null,"k_max_anterior_diopters":null},"belin_ambrosio":{"d":null,"art_max":null}}}}

Use a lateralidade impressa no documento. Em "Ectasia Reforçada Belin / Ambrósio", leia D no rodapé direito e ARTmax no quadro "Índice de Progressão". Não confunda ARTmax com os valores Mín, Méd ou Máx.`

type uploadedFile struct {
	Metadata intakeFile
	Data     []byte
}

type openAIResponse struct {
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		} `json:"content"`
	} `json:"output"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func readIntakeFiles(headers []*multipart.FileHeader) ([]uploadedFile, error) {
	files := make([]uploadedFile, 0, len(headers))
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(data)
		files = append(files, uploadedFile{
			Metadata: intakeFile{Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Size: header.Size, SHA256: hex.EncodeToString(digest[:])},
			Data:     data,
		})
	}
	return files, nil
}

func intakeMetadata(files []uploadedFile) []intakeFile {
	result := make([]intakeFile, len(files))
	for index, file := range files {
		result[index] = file.Metadata
	}
	return result
}

func extractPatient(ctx context.Context, files []uploadedFile) (map[string]any, error) {
	output, err := requestOpenAIJSON(ctx, files, extractionPrompt, 40000)
	if err != nil {
		return nil, err
	}
	analysis, err := decodeAnalysis(output)
	if err != nil {
		return nil, err
	}
	if repairFiles := pentacamFilesNeedingRepair(analysis, files); len(repairFiles) > 0 {
		if repairOutput, repairErr := requestOpenAIJSON(ctx, repairFiles, pentacamRepairPrompt, 5000); repairErr == nil {
			var repair map[string]any
			if json.Unmarshal([]byte(repairOutput), &repair) == nil {
				mergePentacamRepair(analysis, repair)
			}
		}
	}
	enrichSourceFiles(analysis, files)
	return analysis, nil
}

func requestOpenAIJSON(ctx context.Context, files []uploadedFile, prompt string, maxOutputTokens int) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", errors.New("extração indisponível: OPENAI_API_KEY não configurada")
	}

	content := make([]map[string]any, 0, len(files)*2+1)
	content = append(content, map[string]any{"type": "input_text", "text": prompt})
	for _, file := range files {
		content = append(content, map[string]any{"type": "input_text", "text": "Arquivo seguinte: " + file.Metadata.Filename})
		dataURL := "data:" + file.Metadata.ContentType + ";base64," + base64.StdEncoding.EncodeToString(file.Data)
		if strings.HasPrefix(file.Metadata.ContentType, "image/") {
			content = append(content, map[string]any{"type": "input_image", "image_url": dataURL, "detail": "high"})
		} else {
			input := map[string]any{"type": "input_file", "filename": file.Metadata.Filename, "file_data": dataURL}
			if file.Metadata.ContentType == "application/pdf" {
				input["detail"] = "high"
			}
			content = append(content, input)
		}
	}

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-5.4-mini"
	}
	body, err := json.Marshal(map[string]any{
		"model":             model,
		"store":             false,
		"max_output_tokens": maxOutputTokens,
		"input": []map[string]any{{
			"role":    "user",
			"content": content,
		}},
		"text": map[string]any{"format": map[string]any{"type": "json_object"}},
	})
	if err != nil {
		return "", fmt.Errorf("não foi possível preparar a extração: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("não foi possível preparar a extração: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return "", fmt.Errorf("falha ao processar os documentos: %w", err)
	}
	defer response.Body.Close()

	var result openAIResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 10<<20)).Decode(&result); err != nil {
		return "", errors.New("resposta inválida do serviço de extração")
	}
	if response.StatusCode >= 300 {
		if result.Error != nil && result.Error.Message != "" {
			return "", fmt.Errorf("falha na extração: %s", result.Error.Message)
		}
		return "", fmt.Errorf("falha na extração (HTTP %d)", response.StatusCode)
	}
	if result.Status != "completed" {
		return "", errors.New("a extração não foi concluída")
	}

	var output string
	for _, item := range result.Output {
		for _, part := range item.Content {
			if part.Refusal != "" {
				return "", fmt.Errorf("extração recusada: %s", part.Refusal)
			}
			if part.Type == "output_text" {
				output += part.Text
			}
		}
	}
	return output, nil
}

func pentacamFilesNeedingRepair(analysis map[string]any, files []uploadedFile) []uploadedFile {
	exam := pentacamExam(analysis)
	if exam == nil || !pentacamNeedsRepair(exam) {
		return nil
	}

	wanted := map[string]bool{}
	if sources, ok := exam["source"].([]any); ok {
		for _, source := range sources {
			wanted[filepath.Base(fmt.Sprint(source))] = true
		}
	}
	result := make([]uploadedFile, 0, len(wanted))
	for _, file := range files {
		if wanted[file.Metadata.Filename] {
			result = append(result, file)
		}
	}
	return result
}

func pentacamExam(analysis map[string]any) map[string]any {
	exams, _ := analysis["exams"].(map[string]any)
	exam, _ := exams["pentacam_corneal_tomography"].(map[string]any)
	return exam
}

func pentacamNeedsRepair(exam map[string]any) bool {
	eyes, _ := exam["eyes"].(map[string]any)
	for _, eyeName := range []string{"OD", "OS"} {
		eye, _ := eyes[eyeName].(map[string]any)
		if !hasNumberAtAnyPath(eye,
			[]string{"pachymetry", "thinnest_um"},
			[]string{"pachymetry", "point_and_finest_um"},
			[]string{"general", "thinnest_pachy_um"},
			[]string{"general", "pachymetry_thinnest_um"},
		) || !hasNumberAtAnyPath(eye,
			[]string{"anterior_cornea", "kmax_d"},
			[]string{"pachymetry", "k_max_anterior_diopters"},
			[]string{"general", "k_max_anterior_diopters"},
		) || !hasNumberAtAnyPath(eye,
			[]string{"belin_ambrosio", "d"},
			[]string{"ectasia_reforcada_belin_ambrosio", "d"},
		) || !hasNumberAtAnyPath(eye,
			[]string{"belin_ambrosio", "art_max"},
			[]string{"belin_ambrosio", "indice_de_progressao", "art_max"},
			[]string{"ectasia_reforcada_belin_ambrosio", "art_max"},
			[]string{"ectasia_reforcada_belin_ambrosio", "indice_de_progressao", "art_max"},
		) {
			return true
		}
	}
	return false
}

func hasNumberAtAnyPath(root map[string]any, paths ...[]string) bool {
	for _, path := range paths {
		var value any = root
		for _, key := range path {
			object, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = object[key]
		}
		if _, ok := value.(float64); ok {
			return true
		}
	}
	return false
}

func mergePentacamRepair(analysis, repair map[string]any) {
	exam := pentacamExam(analysis)
	if exam == nil {
		return
	}
	targetEyes, _ := exam["eyes"].(map[string]any)
	repairEyes, _ := repair["eyes"].(map[string]any)
	if targetEyes == nil || repairEyes == nil {
		return
	}
	for _, eyeName := range []string{"OD", "OS"} {
		targetEye, _ := targetEyes[eyeName].(map[string]any)
		repairEye, _ := repairEyes[eyeName].(map[string]any)
		if targetEye != nil && repairEye != nil {
			mergeMissingValues(targetEye, repairEye)
		}
	}
}

func mergeMissingValues(target, repair map[string]any) {
	for key, value := range repair {
		if value == nil {
			continue
		}
		if repairObject, ok := value.(map[string]any); ok {
			targetObject, _ := target[key].(map[string]any)
			if targetObject == nil {
				targetObject = map[string]any{}
				target[key] = targetObject
			}
			mergeMissingValues(targetObject, repairObject)
			continue
		}
		if existing, ok := target[key]; !ok || existing == nil {
			target[key] = value
		}
	}
}

func decodeAnalysis(raw string) (map[string]any, error) {
	if raw == "" {
		return nil, errors.New("o serviço de extração não retornou um JSON válido")
	}
	var analysis map[string]any
	if json.Unmarshal([]byte(raw), &analysis) != nil || analysis == nil {
		return nil, errors.New("o serviço de extração não retornou um JSON válido")
	}
	dropMalformedOptionalExams(analysis)
	normalized, err := json.Marshal(analysis)
	if err != nil {
		return nil, errors.New("o serviço de extração não retornou um JSON válido")
	}
	if err := validatePatientJSON(string(normalized)); err != nil {
		return nil, err
	}
	return analysis, nil
}

// A resposta de um exame isolado pode conter um payload vazio para outros
// exames. Exame ausente é válido; exame com payload inválido deve ser ignorado
// para não bloquear a análise do documento que realmente foi enviado.
func dropMalformedOptionalExams(analysis map[string]any) {
	exams, ok := analysis["exams"].(map[string]any)
	if !ok {
		return
	}
	for key, rawExam := range exams {
		if !officialExamKeys[key] {
			continue
		}
		exam, ok := rawExam.(map[string]any)
		if !ok {
			delete(exams, key)
			continue
		}
		if _, ok := exam["source"].([]any); !ok {
			delete(exams, key)
		}
	}
}

func enrichSourceFiles(analysis map[string]any, files []uploadedFile) {
	extracted, _ := analysis["source_files"].([]any)
	sources := make([]any, 0, len(files))
	for index, file := range files {
		source := findSource(extracted, file.Metadata.Filename)
		source["index"] = index
		source["path"] = file.Metadata.Filename
		source["type"] = file.Metadata.ContentType
		source["size_bytes"] = file.Metadata.Size
		source["sha256"] = file.Metadata.SHA256
		sources = append(sources, source)
	}
	analysis["source_files"] = sources
	if _, ok := analysis["schema_version"]; !ok {
		analysis["schema_version"] = "1.0"
	}
	if _, ok := analysis["generated_on"]; !ok {
		analysis["generated_on"] = time.Now().Format("2006-01-02")
	}
	if _, ok := analysis["language"]; !ok {
		analysis["language"] = "pt-BR"
	}
}

func findSource(sources []any, filename string) map[string]any {
	for _, item := range sources {
		source, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path, _ := source["path"].(string)
		name, _ := source["filename"].(string)
		if filepath.Base(path) == filename || name == filename {
			return source
		}
	}
	return map[string]any{}
}

func validateStoredAnalysis(analysis map[string]any, files []intakeFile) error {
	sources, ok := analysis["source_files"].([]any)
	if !ok || len(sources) != len(files) {
		return errors.New("os arquivos não correspondem ao JSON analisado")
	}
	for index, file := range files {
		source, ok := sources[index].(map[string]any)
		if !ok || source["sha256"] != file.SHA256 || filepath.Base(fmt.Sprint(source["path"])) != file.Filename || !strings.HasPrefix(file.Key, "drafts/") {
			return errors.New("os arquivos não correspondem ao JSON analisado")
		}
	}
	return nil
}

func setStoredPaths(analysis map[string]any, files []intakeFile) {
	sources, _ := analysis["source_files"].([]any)
	for index, file := range files {
		if index >= len(sources) {
			return
		}
		if source, ok := sources[index].(map[string]any); ok {
			source["path"] = file.Key
		}
	}
}
