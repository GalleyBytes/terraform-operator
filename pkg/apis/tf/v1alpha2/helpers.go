package v1alpha2

func ListContainsTask(list []TaskName, item TaskName) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}
