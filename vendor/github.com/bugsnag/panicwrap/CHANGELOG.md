## 1.3.1 (2021-01-12)

This release removes the dependency on [osext](https://github.com/kardianos/osext)
for everyone running go1.8+.

### Bug fixes

* Fix windows support by removing undefined syscall

## 1.3.0 (2021-01-05)

### Features

* Support capturing fatal errors from concurrent map writes, nil goroutines,
  out of memory errors, stack exhaustion, and others which use a different panic
  output format.

## 1.2.2 (2020-12-17)

### Bug fixes

* Fix compatibility with go1.7-1.8 by removing dependency on "math/bits" package

## 1.2.1 (2020-12-04)

### Bug fixes

* Fix compatibility with solaris and friends (AIX, etc)
  [Till Wegmüller](https://github.com/Toasterson)
  [#11](https://github.com/bugsnag/panicwrap/pull/11)

## 1.2.0 (2017-08-08)

### Bug fixes

* Fix bug where the program would relaunch without the panic wrapper
  [emersion](https://github.com/emersion)
  [#3](https://github.com/bugsnag/panicwrap/pull/3)

* Fix Solaris build
  [Brian Meyers](https://github.com/bmeyers22)
  [#4](https://github.com/bugsnag/panicwrap/pull/4)

## 1.1.0 (2016-01-18)

* Add ARM64 support
  [liusdu](https://github.com/liusdu)
  [#1](https://github.com/bugsnag/panicwrap/pull/1)

## 1.0.0 (2014-11-10)

### Enhancements

* Add ability to monitor a process
