package utils

// IfThenElse : ternary if macro (generics when)
func IfThenElse(cond bool, valueIfTrue string, valueIfFalse string) string {
	if cond {
		return valueIfTrue
	}
	return valueIfFalse
}

// ExitIfError : fatal error macro, used in initialisations or assertions
func ExitIfError(err error) {
	if err != nil {
		panic(err)
	}
}
