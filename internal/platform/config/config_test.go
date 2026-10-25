package config

import (
	"testing"
)

func TestSelectMongoURI(t *testing.T) {
	tests := []struct {
		name       string
		docker     string
		atlas      string
		wantURI    string
		wantSource MongoSource
		wantErr    bool
	}{
		{
			name:       "docker definida y bien formada: se usa docker",
			docker:     "mongodb://usuario:clave@127.0.0.1:27017/kenko_eps",
			atlas:      "mongodb+srv://usuario:clave@cluster.mongodb.net/kenko_eps",
			wantURI:    "mongodb://usuario:clave@127.0.0.1:27017/kenko_eps",
			wantSource: MongoDocker,
		},
		{
			name:       "docker vacía: se usa atlas",
			docker:     "",
			atlas:      "mongodb+srv://usuario:clave@cluster.mongodb.net/kenko_eps",
			wantURI:    "mongodb+srv://usuario:clave@cluster.mongodb.net/kenko_eps",
			wantSource: MongoAtlas,
		},
		{
			name:       "docker mal formada: se usa atlas",
			docker:     "http://localhost:27017",
			atlas:      "mongodb+srv://usuario:clave@cluster.mongodb.net/kenko_eps",
			wantURI:    "mongodb+srv://usuario:clave@cluster.mongodb.net/kenko_eps",
			wantSource: MongoAtlas,
		},
		{
			name:    "ninguna válida: error",
			docker:  "",
			atlas:   "",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			uri, source, err := selectMongoURI(test.docker, test.atlas)

			if test.wantErr {
				if err == nil {
					t.Fatalf("se esperaba error, se obtuvo URI %q", uri)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if uri != test.wantURI {
				t.Errorf("uri = %q, se quiere %q", uri, test.wantURI)
			}
			if source != test.wantSource {
				t.Errorf("source = %q, se quiere %q", source, test.wantSource)
			}
		})
	}
}
