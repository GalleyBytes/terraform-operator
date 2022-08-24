package v1alpha2

func ListContainsRunType(list []TaskType, item TaskType) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}
