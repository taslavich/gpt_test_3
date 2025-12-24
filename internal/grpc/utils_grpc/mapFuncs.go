package utils

func ValuesSet(siteIdCommon map[string]string) map[string]struct{} {
	domainCommonSet := make(map[string]struct{})
	for _, v := range siteIdCommon {
		domainCommonSet[v] = struct{}{}
	}
	return domainCommonSet
}
