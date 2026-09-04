package docker

// databaseTuning is shared by every database variant; each flag was verified
// against both MariaDB 11.8 and MySQL 8.4.
var databaseTuning = []string{
	"--sql_mode=STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION",
	"--log_bin_trust_function_creators=1",
	"--binlog_cache_size=16M",
	"--key_buffer_size=0",
	"--join_buffer_size=1024M",
	"--innodb_log_file_size=128M",
	"--innodb_buffer_pool_size=1024M",
	"--innodb_buffer_pool_instances=1",
	"--group_concat_max_len=320000",
	"--default-time-zone=+00:00",
	"--max_binlog_size=512M",
	"--binlog_expire_logs_seconds=86400",
}

// buildDatabase builds the database for the selected variant. The tuning
// flags are shared; image, environment prefix, healthcheck client and data
// volume follow the variant. Each variant keeps its own volume so switching
// never points one server at the other's data directory.
func buildDatabase(e *Environment, svc service) composeService {
	variant, version := svc.selected(e)

	envPrefix, adminClient, volume := "MARIADB_", "mariadb-admin", "db-data"
	if variant.Name == DatabaseMySQL {
		envPrefix, adminClient, volume = "MYSQL_", "mysqladmin", "db-data-mysql"
	}

	return composeService{
		Image:   variant.Image + ":" + version,
		Command: databaseTuning,
		Environment: yamlMap[string]{}.
			set(envPrefix+"DATABASE", "shopware").
			set(envPrefix+"ROOT_PASSWORD", "root").
			set(envPrefix+"USER", "shopware").
			set(envPrefix+"PASSWORD", "shopware"),
		Volumes: []string{volume + ":/var/lib/mysql:rw"},
		Healthcheck: &composeHealthcheck{
			Test:          []string{"CMD", adminClient, "ping", "-h", "localhost", "-proot"},
			StartPeriod:   "10s",
			StartInterval: "3s",
			Interval:      "5s",
			Timeout:       "1s",
			Retries:       10,
		},
	}
}
