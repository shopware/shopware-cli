package mysqldump

import (
	"time"

	"github.com/go-sql-driver/mysql"
)

type Config struct {
	Host     string
	Port     string
	Database string
	User     string
	Pass     string
}

func NewConfig(
	user string,
	pass string,
	host string,
	port string,
	database string,
) Config {
	return Config{
		Host:     host,
		Port:     port,
		Database: database,
		User:     user,
		Pass:     pass,
	}
}

func (c *Config) ConnectionString() string {
	var config = mysql.Config{
		Loc:                  time.UTC,
		DBName:               c.Database,
		User:                 c.User,
		Passwd:               c.Pass,
		Net:                  "tcp",
		Addr:                 c.Host + ":" + c.Port,
		ParseTime:            false,
		AllowNativePasswords: true,
		CheckConnLiveness:    true,
	}

	return config.FormatDSN()
}
