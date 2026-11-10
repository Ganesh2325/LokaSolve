package auth

import "time"

func seconds(value int) time.Duration {
	return time.Duration(value) * time.Second
}
