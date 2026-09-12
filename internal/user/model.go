package user

type User struct {
	ID        uint64  `json:"id"`
	LoginID   string  `json:"login_id"`
	Email     string  `json:"email"`
	FullName  string  `json:"full_name"`
	AvatarURL *string `json:"avatar_url,omitempty"`
	Bio       *string `json:"bio,omitempty"`
	Role      string  `json:"role"`
	Status    string  `json:"status"`
}

type ListFilter struct {
	Role   string
	Status string
}

type CreateInput struct {
	BirthPlace *string `json:"birth_place"`
	BirthDate  *string `json:"birth_date"`
	Phone      *string `json:"phone"`
	LoginID    string  `json:"login_id"`
	Email      string  `json:"email"`
	Password   string  `json:"password"`
	FullName   string  `json:"full_name"`
	Role       string  `json:"role"`
	Status     string  `json:"status"`
	NIS        *string `json:"nis"`
	NISN       *string `json:"nisn"`
	NIK        *string `json:"nik"`
	NUPTK      *string `json:"nuptk"`
	EmployeeID *string `json:"employee_id"`
}

type UpdateStatusInput struct {
	Status string `json:"status"`
}
