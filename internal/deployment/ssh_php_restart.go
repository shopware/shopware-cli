package deployment

import _ "embed"

//go:embed ssh_php_restart.php
var sshPHPRestartScript string
