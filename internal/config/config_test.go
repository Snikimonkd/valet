package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	const (
		domain string = "valet.example"
		dsn    string = "postgres://postgres@localhost:5432/valet"
	)

	tests := []struct {
		name    string
		domain  string
		dsn     string
		want    Config
		wantErr string
	}{
		{name: "all set", domain: domain, dsn: dsn, want: Config{Domain: domain, PgDSN: dsn}},
		{name: "no domain", dsn: dsn, wantErr: envDomain},
		{name: "no dsn", domain: domain, wantErr: envPgDSN},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envDomain, tt.domain)
			t.Setenv(envPgDSN, tt.dsn)

			got, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("err = %v, want mention of %s", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("Load() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}
