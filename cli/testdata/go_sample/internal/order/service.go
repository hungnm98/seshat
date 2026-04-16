package order

type Repository struct{}

func (Repository) Save() {}

type Service struct {
	repo Repository
}

func (s Service) Create() {
	s.validate()
	s.repo.Save()
}

func (s Service) validate() {}
