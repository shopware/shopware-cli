package docker

import "fmt"

const (
	storageAccessKey = "shopware"
	storageSecretKey = "shopware"
	storageRcImage   = "rustfs/rc:latest"
)

// buildStorage builds the S3-compatible object store a PaaS lock
// (shopware/k8s-meta) needs. In plain mode the S3 API (9000) and the console
// (9001) are published on the host; in proxy mode they are routed at
// s3.<host> and storage.<host> so media URLs stay HTTPS on the local domain.
func buildStorage(*Environment, service) composeService {
	return composeService{
		Image: "rustfs/rustfs:latest",
		Environment: yamlMap[string]{}.
			set("RUSTFS_VOLUMES", "/data").
			set("RUSTFS_ADDRESS", "0.0.0.0:9000").
			set("RUSTFS_CONSOLE_ADDRESS", "0.0.0.0:9001").
			set("RUSTFS_CONSOLE_ENABLE", "true").
			set("RUSTFS_ACCESS_KEY", storageAccessKey).
			set("RUSTFS_SECRET_KEY", storageSecretKey),
		Volumes: []string{"rustfs-data:/data"},
		Healthcheck: &composeHealthcheck{
			Test:          []string{"CMD", "curl", "-f", "http://127.0.0.1:9000/health"},
			StartPeriod:   "20s",
			StartInterval: "3s",
			Interval:      "5s",
			Timeout:       "5s",
			Retries:       10,
		},
	}
}

// buildStorageInit builds the one-shot container that creates the shop's
// buckets once the storage is healthy; web waits for it to complete.
func buildStorageInit(*Environment, service) composeService {
	return composeService{
		Image:      storageRcImage,
		Entrypoint: []string{"/bin/sh", "-c"},
		Command: []string{fmt.Sprintf(
			"rc alias set storage http://storage:9000 %s %s && rc mb --ignore-existing storage/shopware-private && rc mb --ignore-existing storage/shopware-public && rc anonymous set download storage/shopware-public",
			storageAccessKey, storageSecretKey,
		)},
		DependsOn: yamlMap[composeDependency]{}.
			set(ServiceStorage, composeDependency{Condition: "service_healthy"}),
	}
}
