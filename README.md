# powermon

This is a little utility to monitor power state changes between AC and BATTERY,
running a command any time there is a state transition. This allows you to
devise a script that configures whatever default settings you want for power
saving while on battery and switch back to full performance when plugged in.

The script that is executed should accept a single argument, which will be one
of "UNKNOWN", "ON_BATTERY" or "AC_POWER".

## Flags

- action
  - an executable to run, which accepts a single parameter
  - environment variable expansion is done on the value of the string

- logfile
  - a path to send log output to

- verbose
  - enable logging


## License

powermon is available under the Simplified BSD License; see LICENSE for
the full text.
