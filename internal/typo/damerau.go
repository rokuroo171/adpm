package typo

func distance(source, target string) int {
	a, b := []rune(source), []rune(target)
	maxDistance := len(a) + len(b)
	width := len(b) + 2
	matrix := make([]int, (len(a)+2)*width)
	cell := func(row, column int) int { return row*width + column }
	matrix[cell(0, 0)] = maxDistance
	for i := 0; i <= len(a); i++ {
		matrix[cell(i+1, 0)] = maxDistance
		matrix[cell(i+1, 1)] = i
	}
	for j := 0; j <= len(b); j++ {
		matrix[cell(0, j+1)] = maxDistance
		matrix[cell(1, j+1)] = j
	}
	lastRow := make(map[rune]int)
	for i := 1; i <= len(a); i++ {
		lastMatchColumn := 0
		for j := 1; j <= len(b); j++ {
			previousRow := lastRow[b[j-1]]
			previousColumn := lastMatchColumn
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
				lastMatchColumn = j
			}
			matrix[cell(i+1, j+1)] = min(
				matrix[cell(i, j)]+cost,
				matrix[cell(i+1, j)]+1,
				matrix[cell(i, j+1)]+1,
				matrix[cell(previousRow, previousColumn)]+i-previousRow-1+1+j-previousColumn-1,
			)
		}
		lastRow[a[i-1]] = i
	}
	return matrix[cell(len(a)+1, len(b)+1)]
}
