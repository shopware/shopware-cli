package deployment

import _ "embed"

//go:embed ssh_permissions.php
var sshPermissionsScript string
