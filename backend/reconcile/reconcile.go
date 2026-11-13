// Package reconcile finds the drift the commit protocol (no distributed
// transaction) can leave behind. It is REPORT-ONLY: it deletes nothing.
//
//	garage-case-without-sql    case folder with a marker but no patients.case_id row
//	sql-case-without-marker    patients.case_id whose case has no paciente_compilado.json
//	case-without-marker        case folder without paciente_compilado.json
//	unreferenced-object        object in a case folder not referenced by the marker
//	exam-without-object        exams row whose object is missing in Garage
//
// Findings carry case ids only (no patient names).
package reconcile

import (
	"context"
	"encoding/json"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gorm.io/gorm"

	"refratia/backend/database"
	"refratia/backend/fileStorage"
)

const casesPrefix = "refratia/cases/"

type Finding struct{ Kind, Detail string }

func Run(ctx context.Context, db *gorm.DB, client fileStorage.Repository, bucket string) ([]Finding, error) {
	objects := map[string][]string{}
	var token *string
	for {
		listed, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(casesPrefix), ContinuationToken: token})
		if err != nil {
			return nil, err
		}
		for _, object := range listed.Contents {
			key := aws.ToString(object.Key)
			rest := key[len(casesPrefix):]
			caseID := rest
			for i := 0; i < len(rest); i++ {
				if rest[i] == '/' {
					caseID = rest[:i]
					break
				}
			}
			objects[caseID] = append(objects[caseID], key)
		}
		if !aws.ToBool(listed.IsTruncated) {
			break
		}
		token = listed.NextContinuationToken
	}

	var linked []string
	if err := db.Model(&database.Patient{}).Where("case_id <> ''").Pluck("case_id", &linked).Error; err != nil {
		return nil, err
	}
	inSQL := map[string]bool{}
	for _, id := range linked {
		inSQL[id] = true
	}

	var findings []Finding
	report := func(kind, detail string) { findings = append(findings, Finding{kind, detail}) }

	existing := map[string]bool{}
	for caseID, keys := range objects {
		marker := casesPrefix + caseID + "/paciente_compilado.json"
		hasMarker := false
		for _, key := range keys {
			existing[key] = true
			hasMarker = hasMarker || key == marker
		}
		if !hasMarker {
			report("case-without-marker", caseID)
			continue
		}
		if !inSQL[caseID] {
			report("garage-case-without-sql", caseID)
		}
		referenced := referencedPaths(ctx, client, bucket, marker)
		for _, key := range keys {
			if key != marker && !referenced[key] {
				base := path.Base(key)
				report("unreferenced-object", caseID+"\t"+base[:min(len(base), 12)]+"…")
			}
		}
	}
	for _, id := range linked {
		if _, ok := objects[id]; !ok {
			report("sql-case-without-marker", id)
		}
	}

	var exams []database.Exam
	if err := db.Select("case_id", "object_key").Find(&exams).Error; err != nil {
		return nil, err
	}
	for _, exam := range exams {
		if !existing[exam.ObjectKey] {
			report("exam-without-object", exam.CaseID)
		}
	}
	return findings, nil
}

func referencedPaths(ctx context.Context, client fileStorage.Repository, bucket, marker string) map[string]bool {
	result := map[string]bool{}
	object, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(marker)})
	if err != nil {
		return result
	}
	defer object.Body.Close()
	var manifest struct {
		SourceFiles []struct {
			Path string `json:"path"`
		} `json:"source_files"`
	}
	if json.NewDecoder(object.Body).Decode(&manifest) == nil {
		for _, source := range manifest.SourceFiles {
			result[source.Path] = true
		}
	}
	return result
}
