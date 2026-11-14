package intake

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
	if _, errorValue := mergeFileCheckpointAnalysis(identityAnalysis("DMY"), identityAnalysis("MDY")); errorValue != nil {
		t.Fatalf("swapped day/month must merge: %v", errorValue)
	}
	other := identityAnalysis("MDY")
	other["patient"].(map[string]any)["full_name"] = "BIA LIMA COSTA"
	other["verificacao_identidade"].([]any)[0].(map[string]any)["nome_lido"] = "BIA LIMA COSTA"
	if _, errorValue := mergeFileCheckpointAnalysis(identityAnalysis("DMY"), other); errorValue == nil {
		t.Fatal("different patients must not merge")
	}
}
