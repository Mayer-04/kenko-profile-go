package config

import (
	"testing"
)

func withEnv(t *testing.T, k, v string) {
	t.Helper()
	t.Setenv(k, v)
}

func TestURISelection(t *testing.T) {
	tests := []struct {
		name       string
		dockerURI  string
		atlasURI   string
		wantSource MongoSource
		wantErr    bool
	}{
		{
			name:       "docker primero",
			dockerURI:  "mongodb://user:pass@mongo:27017/db?authSource=db",
			atlasURI:   "mongodb+srv://u:p@host/db?retryWrites=true",
			wantSource: MongoSourceDocker,
		},
		{
			name:       "atlas como respaldo",
			dockerURI:  "",
			atlasURI:   "mongodb+srv://u:p@host/db?retryWrites=true",
			wantSource: MongoSourceAtlas,
		},
		{
			name:      "rechaza URIs mal formadas",
			dockerURI: "postgres://x",
			atlasURI:  "",
			wantErr:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withEnv(t, "MONGODB_URI", tc.dockerURI)
			withEnv(t, "MONGO_ATLAS_URI", tc.atlasURI)
			withEnv(t, "PROFILE_QUEUE_URL", "https://sqs.us-east-1.amazonaws.com/1/q")
			cfg, err := Load()
			if tc.wantErr {
				if err == nil {
					t.Fatal("URIs mal formadas deberían fallar")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.MongoSource != tc.wantSource {
				t.Fatalf("fuente = %q, quiero %q", cfg.MongoSource, tc.wantSource)
			}
		})
	}
}
