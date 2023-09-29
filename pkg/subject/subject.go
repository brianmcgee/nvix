package subject

var subjectPrefix = "TVIX"

func SetPrefix(prefix string) {
	subjectPrefix = prefix
}

func GetPrefix() string {
	return subjectPrefix
}

func WithPrefix(subj string) string {
	return subjectPrefix + "." + subj
}
