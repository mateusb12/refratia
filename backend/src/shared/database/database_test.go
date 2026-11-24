package database

import "testing"

func TestResolvedRefusesConflictingDatabaseNames(t *testing.T) {
	cases := []struct {
		name, url, explicit, want string
		wantErr                   bool
	}{
		{name: "production split", url: "postgres://u:p@db:5432/postgres?sslmode=disable", explicit: "refratia", wantErr: true},
		{name: "agreeing", url: "postgres://u:p@db:5432/refratia", explicit: "refratia", want: "refratia"},
		{name: "url only names the database", url: "postgres://u:p@db:5432/other", want: "other"},
		{name: "url without database", url: "postgres://u:p@db:5432", explicit: "refratia", wantErr: true},
		{name: "no url keeps POSTGRES_DB", explicit: "refratia", want: "refratia"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			config := Config{URL: testCase.url, ExplicitName: testCase.explicit, Name: defaultDatabase}
			if testCase.explicit != "" {
				config.Name = testCase.explicit
			}
			got, err := config.resolved()
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got database %q", got.Name)
				}
				return
			}
			if err != nil || got.Name != testCase.want {
				t.Fatalf("got %q, %v; want %q", got.Name, err, testCase.want)
			}
		})
	}
}
