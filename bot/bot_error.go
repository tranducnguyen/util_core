package bot

type STATUS int

const (
	Bad STATUS = iota
	Hit
	Retries
	ToCheck
	Cancel
	Error
)

type BotError struct {
	Type STATUS
}

var StatusDescription = map[STATUS]string{

	Bad:     "Thất bại",
	Hit:     "Thành công",
	Retries: "Thử lại",
	ToCheck: "Kiểm tra",
	Cancel:  "Hủy",
	Error:   "Lỗi",
}
