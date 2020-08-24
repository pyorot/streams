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

// Await : object to await one or many async tasks
// can't be used if the tasks return something
// (will fix this once Go gets generics)
// a task should be created and awaited in the same thread
type Await []chan (bool)

// Add : add task to await object
func (a *Await) Add(ch chan (bool)) {
	*a = append(*a, ch)
}

// Flush : await every task in await object, then clear
func (a *Await) Flush() {
	for _, ch := range *a {
		<-ch
	}
	*a = nil
}
