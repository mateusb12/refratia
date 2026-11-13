package main

import "testing"

func identityAnalysis(order string) map[string]any {
	return map[string]any{
		"patient": map[string]any{"full_name": "ANA PEREIRA SOUZA"},
		"verificacao_identidade": []any{map[string]any{
			"source": "f", "nome_lido": "ANA PEREIRA SOUZA",
			"nascimento_lido": "08/05/1969", "date_order": order,
		}},
	}
}

// Per-document date order makes the same printed date resolve to swapped
// day/month; a cached checkpoint must still merge (protocol 0.B, non-blocking).
func TestMergeCheckpointAcceptsDayMonthSwappedIdentity(t *testing.T) {
	if _, err := mergeFileCheckpointAnalysis(identityAnalysis("DMY"), identityAnalysis("MDY")); err != nil {
		t.Fatalf("swapped day/month must merge: %v", err)
	}
	other := identityAnalysis("MDY")
	other["patient"].(map[string]any)["full_name"] = "BIA LIMA COSTA"
	other["verificacao_identidade"].([]any)[0].(map[string]any)["nome_lido"] = "BIA LIMA COSTA"
	if _, err := mergeFileCheckpointAnalysis(identityAnalysis("DMY"), other); err == nil {
		t.Fatal("different patients must not merge")
	}
}
