package encode

import "fmt"

func (emngr *EncodeManager) BulkInsertSet(key string, values []string) error {
	conn := emngr.redisPool.Get()
	defer conn.Close()

	BatchSize := 5000
	for i := 0; i < len(values); {
		startIndex := i
		endIndex := i + BatchSize
		if endIndex > len(values) {
			endIndex = len(values)
		}

		batch := values[startIndex:endIndex]
		args := make([]interface{}, len(batch)+1)
		args[0] = key
		for i, v := range batch {
			args[i+1] = v
		}
		result, _ := conn.Do("SADD", args...)
		fmt.Println(result)
		i = i + BatchSize
	}
	return nil
}
