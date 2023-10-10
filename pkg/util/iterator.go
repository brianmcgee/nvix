package util

type Iterator[T any] interface {
	Next() (T, error)
	Close() error
}
