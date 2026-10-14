package clickhouse_loader

func uint8Bool(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}
