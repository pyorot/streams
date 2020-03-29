package main

// ternary if macro
func ifThenElse(cond bool, valueIfTrue interface{}, valueIfFalse interface{}) interface{} {
	if cond {
		return valueIfTrue
	}
	return valueIfFalse
}

// fatal error macro, used in initialisations
func exitIfError(err error) {
	if err != nil {
		panic(err)
	}
}
