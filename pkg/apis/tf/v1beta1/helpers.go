package v1beta1

func ListContainsTask(list []TaskName, item TaskName) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}

func TaskListsAreEqual(l1, l2 []TaskName) bool {
	if len(l1) != len(l2) {
		return false
	}
	for i, _ := range l1 {
		if l1[i].String() != l2[i].String() {
			return false
		}
	}
	return true
}
