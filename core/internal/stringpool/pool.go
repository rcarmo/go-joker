package stringpool

type Pool map[string]*string

func (p Pool) Intern(s string) *string {
	if ss, exists := p[s]; exists {
		return ss
	}
	p[s] = &s
	return &s
}
