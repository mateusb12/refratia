// Command reconcile is REPORT-ONLY: it deletes nothing. It lists the drift the
// commit protocol (no distributed transaction) can leave behind:
//
//	garage-case-without-sql    case folder with a marker but no patients.case_id row
//	sql-case-without-marker    patients.case_id whose case has no paciente_compilado.json
//	unreferenced-object        object in a case folder not referenced by the marker
//	exam-without-object        exams row whose object is missing in Garage
//
// Run against the environment you load; with REFRATIA_NO_DOTENV=1 it never
// reads the developer .env. Prints case ids and counts only (no patient names).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"refratia/backend/database"
	"refratia/backend/fileStorage"
)

const casesPrefix = "refratia/cases/"

func main() {
	ctx := context.Background()
	db, err := database.Open(ctx)
	check(err)
	defer db.Close()
	client, err := fileStorage.New(ctx)
	check(err)
	bucket := os.Getenv("BUCKET_NAME")

	// Garage: case id -> object keys.
	objects := map[string][]string{}
	var token *string
	for {
		listed, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(casesPrefix), ContinuationToken: token})
		check(err)
		for _, object := range listed.Contents {
			key := aws.ToString(object.Key)
			caseID, _, _ := strings.Cut(strings.TrimPrefix(key, casesPrefix), "/")
			objects[caseID] = append(objects[caseID], key)
		}
		if !aws.ToBool(listed.IsTruncated) {
			break
		}
		token = listed.NextContinuationToken
	}

	var linked []string
	check(db.Gorm.Model(&database.Patient{}).Where("case_id <> ''").Pluck("case_id", &linked).Error)
	inSQL := map[string]bool{}
	for _, id := range linked {
		inSQL[id] = true
	}

	problems := 0
	report := func(kind, detail string) { problems++; fmt.Printf("%s\t%s\n", kind, detail) }

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
				report("unreferenced-object", caseID+"\t"+path.Base(key)[:min(len(path.Base(key)), 12)]+"…")
			}
		}
	}
	for _, id := range linked {
		if _, ok := objects[id]; !ok {
			report("sql-case-without-marker", id)
		}
	}

	var exams []database.Exam
	check(db.Gorm.Select("case_id", "object_key").Find(&exams).Error)
	for _, exam := range exams {
		if !existing[exam.ObjectKey] {
			report("exam-without-object", exam.CaseID)
		}
	}
	fmt.Printf("%d finding(s)\n", problems)
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

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "reconcile:", err)
		os.Exit(1)
	}
}
