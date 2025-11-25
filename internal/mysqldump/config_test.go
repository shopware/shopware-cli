package mysqldump

import (
	"testing"
)

func TestConfig_ConnectionString(t *testing.T) {
	tests := []struct {
		name string
		conf Config
		want string
	}{
		{
			"Connection Successful",
			NewConfig(
				"root",
				"test123",
				"192.168.55.101",
				"3306",
				"hydra",
			),
			"root:test123@tcp(192.168.55.101:3306)/hydra?maxAllowedPacket=0",
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.conf.ConnectionString(); got != tt.want {
					t.Errorf("Config.ConnectionString() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}
