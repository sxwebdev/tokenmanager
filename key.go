package tokenmanager

const keyPrefix = "tokenmanager:"

func getKey(id string) []byte {
	return []byte(keyPrefix + id)
}
