package poker

//GenCombinations generates, from two natural numbers n > r,
//all the possible combinations of r indexes taken from 0 to n-1.
//For example if n=3 and r=2, the result will be:
//[0,1], [0,2] and [1,2]
func genCombinations(n, r int) <-chan []int {

	if r > n {
		panic("Invalid arguments")
	}

	ch := make(chan []int)

	go func() {
		result := make([]int, r)
		for i := range result {
			result[i] = i
		}

		temp := make([]int, r)
		copy(temp, result) // avoid overwriting of result
		ch <- temp

		for {
			for i := r - 1; i >= 0; i-- {
				if result[i] < i+n-r {
					result[i]++
					for j := 1; j < r-i; j++ {
						result[i+j] = result[i] + j
					}
					temp := make([]int, r)
					copy(temp, result) // avoid overwriting of result
					ch <- temp
					break
				}
			}
			if result[0] >= n-r {
				break
			}
		}
		close(ch)

	}()
	return ch
}

//CombinationsInt generates all the combinations of r elements
//extracted from an slice of integers
func combinations(iterable []Card, r int) chan []Card {

	length := len(iterable)
	if length < r {
		panic("Invalid arguments")
	}
	ch := make(chan []Card)

	go func() {

		length := len(iterable)

		for comb := range genCombinations(length, r) {
			result := make([]Card, r)
			for i, val := range comb {
				result[i] = iterable[val]
			}
			ch <- result
		}

		close(ch)
	}()
	return ch
}
