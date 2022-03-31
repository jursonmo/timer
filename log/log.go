package log

import "log"

type Logger interface {
	Debugf(format string, a ...interface{})
	Infof(format string, a ...interface{})
	Warnf(format string, a ...interface{})
	Errorf(format string, a ...interface{})
	Fatalf(format string, a ...interface{})
}

var DefaultLog = (*defaultLog)(nil)

type defaultLog struct{}

func (l *defaultLog) Debugf(format string, a ...interface{}) {
	log.Printf("[debug]"+format, a)
}

func (l *defaultLog) Infof(format string, a ...interface{}) {
	log.Printf("[info]"+format, a)
}
func (l *defaultLog) Warnf(format string, a ...interface{}) {
	log.Printf("[warn]"+format, a)
}
func (l *defaultLog) Errorf(format string, a ...interface{}) {
	log.Printf("[error]"+format, a)
}
func (l *defaultLog) Fatalf(format string, a ...interface{}) {
	log.Fatalf("[fatal]"+format, a)
}
