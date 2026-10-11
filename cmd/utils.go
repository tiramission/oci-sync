package cmd

import (
	"fmt"
	"strings"

	"charm.land/log/v2"
	"github.com/tiramission/oci-sync/internal/cache"
)

// systemAnnotationPrefix is reserved by the oci-sync manifest annotations.
// The encrypted flag decides whether pull decrypts before unpacking, and the
// version annotation is the only marker list uses to recognise an oci-sync
// artifact, so neither may be set or cleared by a user label.
const systemAnnotationPrefix = "io.oci-sync."

func rejectSystemLabel(key string) error {
	if strings.HasPrefix(key, systemAnnotationPrefix) {
		return fmt.Errorf("label key %q is reserved: the %s* namespace is managed by oci-sync", key, systemAnnotationPrefix)
	}
	return nil
}

// parseLabels turns repeated key=value flags into a map, rejecting malformed
// pairs and reserved keys. When errOnMalformed is false, pairs without '='
// are skipped instead of failing the command.
func parseLabels(labels []string, errOnMalformed bool) (map[string]string, error) {
	out := make(map[string]string, len(labels))
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) < 2 {
			if errOnMalformed {
				return nil, fmt.Errorf("invalid label format %q, expected key=value", l)
			}
			log.Warn("skipping malformed label", "label", l, "expected", "key=value")
			continue
		}
		if err := rejectSystemLabel(parts[0]); err != nil {
			return nil, err
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

// recordActivity logs activity-cache failures instead of dropping them: a
// broken cache must not turn a successful push or pull into a silent no-op
// in the user's history.
func recordActivity(a cache.Activity) {
	if err := cache.AddActivity(a); err != nil {
		log.Warn("cannot record activity history", "path", cache.CacheFilePath(), "error", err)
	}
}

// formatBytes formats a byte count as a human-readable string. It takes
// int64 because every size in this program comes off the wire or off the
// filesystem as an int64; an int parameter would truncate a multi-
// gigabyte artifact on a 32-bit build and invite the same truncation at
// every call site that tried to pre-empt it.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
