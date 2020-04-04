/*
 * The MIT License (MIT)
 *
 * Copyright (c) 2018 Roland Singer [roland.singer@deserbit.com]
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package grumble

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

type parseFunc func(flag, equalVal string, args []string, res FlagMap) ([]string, bool, error)
type defaultFunc func(res FlagMap)

type flagItem struct {
	Short           string
	Long            string
	Help            string
	HelpArgs        string
	HelpShowDefault bool
	Default         interface{}
}

// Flags holds all the registered flags.
type Flags struct {
	parsers  []parseFunc
	defaults map[string]defaultFunc
	list     []*flagItem
}

// sort the flags by their name.
func (f *Flags) sort() {
	sort.Slice(f.list, func(i, j int) bool {
		return f.list[i].Long < f.list[j].Long
	})
}

func (f *Flags) register(
	short, long, help, helpArgs string,
	helpShowDefault bool,
	defaultValue interface{},
	df defaultFunc,
	pf parseFunc,
) {
	// Validate.
	if len(short) > 1 {
		panic(fmt.Errorf("invalid short flag: '%s': must be a single character", short))
	} else if strings.HasPrefix(short, "-") {
		panic(fmt.Errorf("invalid short flag: '%s': must not start with a '-'", short))
	} else if len(long) == 0 {
		panic(fmt.Errorf("empty long flag: short='%s'", short))
	} else if strings.HasPrefix(long, "-") {
		panic(fmt.Errorf("invalid long flag: '%s': must not start with a '-'", long))
	} else if len(help) == 0 {
		panic(fmt.Errorf("empty flag help message for flag: '%s'", long))
	}

	f.list = append(f.list, &flagItem{
		Short:           short,
		Long:            long,
		Help:            help,
		HelpShowDefault: helpShowDefault,
		HelpArgs:        helpArgs,
		Default:         defaultValue,
	})

	if f.defaults == nil {
		f.defaults = make(map[string]defaultFunc)
	}
	f.defaults[long] = df

	f.parsers = append(f.parsers, pf)
}

func (f *Flags) match(flag, short, long string) bool {
	return len(long) > 0 && flag == long
}

func (f *Flags) parse(args []string, res FlagMap) ([]string, error) {

	var err error
	var parsed bool

	// Parse all leading flags.
Loop:
	for len(args) > 0 {
		//Check to see if this is a flag, we can't rely on the prefix - in netgrumble
		a := args[0]

		//See if this is a flag - netgrumble add
		full_param := ""
		//@todo figure out how to allow flags to only be used once per command instead of overwriting
		for _, f := range f.list {
			if len(a) <= len(f.Long) && strings.HasPrefix(f.Long, a) {
				if full_param != "" {
					return nil, errors.New("Ambiguous command flags: " + a + " could mean " + full_param + " or " + f.Long + ".")
				}

				full_param = f.Long

			}
		}
		if full_param == "" {
			break Loop
		}

		a = full_param

		args = args[1:]
		pos := strings.Index(a, "=")
		equalVal := ""
		if pos > 0 {
			equalVal = a[pos+1:]
			a = a[:pos]
		}

		for _, p := range f.parsers {
			args, parsed, err = p(a, equalVal, args, res)
			if err != nil {
				return nil, err
			} else if parsed {
				continue Loop
			}
		}
		return nil, fmt.Errorf("invalid flag: %s", a)
	}

	// Finally set all the default values for not passed flags.
	if f.defaults == nil {
		return args, nil
	}

	for _, i := range f.list {
		if _, ok := res[i.Long]; ok {
			continue
		}
		df, ok := f.defaults[i.Long]
		if !ok {
			return nil, fmt.Errorf("invalid flag: missing default function: %s", i.Long)
		}
		df(res)
	}

	return args, nil
}

// StringL same as String, but without a shorthand.
func (f *Flags) StringL(long, defaultValue, help string) {
	f.String("", long, defaultValue, help)
}

// String registers a string flag.
func (f *Flags) String(short, long, defaultValue, help string) {
	f.register(short, long, help, "string", true, defaultValue,
		func(res FlagMap) {
			res[long] = &FlagMapItem{
				Value:     defaultValue,
				IsDefault: true,
			}
		},
		func(flag, equalVal string, args []string, res FlagMap) ([]string, bool, error) {
			if !f.match(flag, short, long) {
				return args, false, nil
			}
			if len(equalVal) > 0 {
				res[long] = &FlagMapItem{
					Value:     trimQuotes(equalVal),
					IsDefault: false,
				}
				return args, true, nil
			}
			if len(args) == 0 {
				return args, false, fmt.Errorf("missing string value for flag: %s", flag)
			}
			res[long] = &FlagMapItem{
				Value:     args[0],
				IsDefault: false,
			}
			args = args[1:]
			return args, true, nil
		})
}

// BoolL same as Bool, but without a shorthand.
func (f *Flags) BoolL(long string, defaultValue bool, help string) {
	f.Bool("", long, defaultValue, help)
}

// Bool registers a boolean flag.
func (f *Flags) Bool(short, long string, defaultValue bool, help string) {
	f.register(short, long, help, "", false, defaultValue,
		func(res FlagMap) {
			res[long] = &FlagMapItem{
				Value:     defaultValue,
				IsDefault: true,
			}
		},
		func(flag, equalVal string, args []string, res FlagMap) ([]string, bool, error) {
			if !f.match(flag, short, long) {
				return args, false, nil
			}
			if len(equalVal) > 0 {
				b, err := strconv.ParseBool(equalVal)
				if err != nil {
					return args, false, fmt.Errorf("invalid boolean value for flag: %s", flag)
				}
				res[long] = &FlagMapItem{
					Value:     b,
					IsDefault: false,
				}
				return args, true, nil
			}
			res[long] = &FlagMapItem{
				Value:     true,
				IsDefault: false,
			}
			return args, true, nil
		})
}

// IntL same as Int, but without a shorthand.
func (f *Flags) IntL(long string, defaultValue int, help string) {
	f.Int("", long, defaultValue, help)
}

// Int registers an int flag.
func (f *Flags) Int(short, long string, defaultValue int, help string) {
	f.register(short, long, help, "int", true, defaultValue,
		func(res FlagMap) {
			res[long] = &FlagMapItem{
				Value:     defaultValue,
				IsDefault: true,
			}
		},
		func(flag, equalVal string, args []string, res FlagMap) ([]string, bool, error) {
			if !f.match(flag, short, long) {
				return args, false, nil
			}
			var vStr string
			if len(equalVal) > 0 {
				vStr = equalVal
			} else if len(args) > 0 {
				vStr = args[0]
				args = args[1:]
			} else {
				return args, false, fmt.Errorf("missing int value for flag: %s", flag)
			}
			i, err := strconv.Atoi(vStr)
			if err != nil {
				return args, false, fmt.Errorf("invalid int value for flag: %s", flag)
			}
			res[long] = &FlagMapItem{
				Value:     i,
				IsDefault: false,
			}
			return args, true, nil
		})
}

// Int64L same as Int64, but without a shorthand.
func (f *Flags) Int64L(long string, defaultValue int64, help string) {
	f.Int64("", long, defaultValue, help)
}

// Int64 registers an int64 flag.
func (f *Flags) Int64(short, long string, defaultValue int64, help string) {
	f.register(short, long, help, "int", true, defaultValue,
		func(res FlagMap) {
			res[long] = &FlagMapItem{
				Value:     defaultValue,
				IsDefault: true,
			}
		},
		func(flag, equalVal string, args []string, res FlagMap) ([]string, bool, error) {
			if !f.match(flag, short, long) {
				return args, false, nil
			}
			var vStr string
			if len(equalVal) > 0 {
				vStr = equalVal
			} else if len(args) > 0 {
				vStr = args[0]
				args = args[1:]
			} else {
				return args, false, fmt.Errorf("missing int value for flag: %s", flag)
			}
			i, err := strconv.ParseInt(vStr, 10, 64)
			if err != nil {
				return args, false, fmt.Errorf("invalid int value for flag: %s", flag)
			}
			res[long] = &FlagMapItem{
				Value:     i,
				IsDefault: false,
			}
			return args, true, nil
		})
}

// UintL same as Uint, but without a shorthand.
func (f *Flags) UintL(long string, defaultValue uint, help string) {
	f.Uint("", long, defaultValue, help)
}

// Uint registers an uint flag.
func (f *Flags) Uint(short, long string, defaultValue uint, help string) {
	f.register(short, long, help, "uint", true, defaultValue,
		func(res FlagMap) {
			res[long] = &FlagMapItem{
				Value:     defaultValue,
				IsDefault: true,
			}
		},
		func(flag, equalVal string, args []string, res FlagMap) ([]string, bool, error) {
			if !f.match(flag, short, long) {
				return args, false, nil
			}
			var vStr string
			if len(equalVal) > 0 {
				vStr = equalVal
			} else if len(args) > 0 {
				vStr = args[0]
				args = args[1:]
			} else {
				return args, false, fmt.Errorf("missing uint value for flag: %s", flag)
			}
			i, err := strconv.ParseUint(vStr, 10, 64)
			if err != nil {
				return args, false, fmt.Errorf("invalid uint value for flag: %s", flag)
			}
			res[long] = &FlagMapItem{
				Value:     uint(i),
				IsDefault: false,
			}
			return args, true, nil
		})
}

// Uint64L same as Uint64, but without a shorthand.
func (f *Flags) Uint64L(long string, defaultValue uint64, help string) {
	f.Uint64("", long, defaultValue, help)
}

// Uint64 registers an uint64 flag.
func (f *Flags) Uint64(short, long string, defaultValue uint64, help string) {
	f.register(short, long, help, "uint", true, defaultValue,
		func(res FlagMap) {
			res[long] = &FlagMapItem{
				Value:     defaultValue,
				IsDefault: true,
			}
		},
		func(flag, equalVal string, args []string, res FlagMap) ([]string, bool, error) {
			if !f.match(flag, short, long) {
				return args, false, nil
			}
			var vStr string
			if len(equalVal) > 0 {
				vStr = equalVal
			} else if len(args) > 0 {
				vStr = args[0]
				args = args[1:]
			} else {
				return args, false, fmt.Errorf("missing uint value for flag: %s", flag)
			}
			i, err := strconv.ParseUint(vStr, 10, 64)
			if err != nil {
				return args, false, fmt.Errorf("invalid uint value for flag: %s", flag)
			}
			res[long] = &FlagMapItem{
				Value:     i,
				IsDefault: false,
			}
			return args, true, nil
		})
}

// Float64L same as Float64, but without a shorthand.
func (f *Flags) Float64L(long string, defaultValue float64, help string) {
	f.Float64("", long, defaultValue, help)
}

// Float64 registers an float64 flag.
func (f *Flags) Float64(short, long string, defaultValue float64, help string) {
	f.register(short, long, help, "float", true, defaultValue,
		func(res FlagMap) {
			res[long] = &FlagMapItem{
				Value:     defaultValue,
				IsDefault: true,
			}
		},
		func(flag, equalVal string, args []string, res FlagMap) ([]string, bool, error) {
			if !f.match(flag, short, long) {
				return args, false, nil
			}
			var vStr string
			if len(equalVal) > 0 {
				vStr = equalVal
			} else if len(args) > 0 {
				vStr = args[0]
				args = args[1:]
			} else {
				return args, false, fmt.Errorf("missing float value for flag: %s", flag)
			}
			i, err := strconv.ParseFloat(vStr, 64)
			if err != nil {
				return args, false, fmt.Errorf("invalid float value for flag: %s", flag)
			}
			res[long] = &FlagMapItem{
				Value:     i,
				IsDefault: false,
			}
			return args, true, nil
		})
}

// DurationL same as Duration, but without a shorthand.
func (f *Flags) DurationL(long string, defaultValue time.Duration, help string) {
	f.Duration("", long, defaultValue, help)
}

// Duration registers a duration flag.
func (f *Flags) Duration(short, long string, defaultValue time.Duration, help string) {
	f.register(short, long, help, "duration", true, defaultValue,
		func(res FlagMap) {
			res[long] = &FlagMapItem{
				Value:     defaultValue,
				IsDefault: true,
			}
		},
		func(flag, equalVal string, args []string, res FlagMap) ([]string, bool, error) {
			if !f.match(flag, short, long) {
				return args, false, nil
			}
			var vStr string
			if len(equalVal) > 0 {
				vStr = equalVal
			} else if len(args) > 0 {
				vStr = args[0]
				args = args[1:]
			} else {
				return args, false, fmt.Errorf("missing duration value for flag: %s", flag)
			}
			d, err := time.ParseDuration(vStr)
			if err != nil {
				return args, false, fmt.Errorf("invalid duration value for flag: %s", flag)
			}
			res[long] = &FlagMapItem{
				Value:     d,
				IsDefault: false,
			}
			return args, true, nil
		})
}

func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// IPL same as IP, but without a shorthand.
func (f *Flags) IPNL(long string, defaultValue IPandMASK, help string) {
	f.IPN("", long, defaultValue, help)
}

// NETGRUBMLE: IP registers a IP flag.
func (f *Flags) IPN(short string, long string, defaultValue IPandMASK, help string) {
	f.register(short, long, help, "ip", true, defaultValue,
		func(res FlagMap) {
			res[long] = &FlagMapItem{
				Value:     defaultValue,
				IsDefault: true,
			}
		},
		func(flag, equalVal string, args []string, res FlagMap) ([]string, bool, error) {
			if !f.match(flag, short, long) {
				return args, false, nil
			}
			if len(equalVal) > 0 {
				res[long] = &FlagMapItem{
					Value:     trimQuotes(equalVal),
					IsDefault: false,
				}
				return args, true, nil
			}
			if len(args) == 0 {
				return args, false, fmt.Errorf("missing ip value for %s", flag)
			}

			var ip net.IP
			var mask net.IPMask
			var err error

			if strings.Contains(args[0], `/`) {
				var netw *net.IPNet
				ip, netw, err = net.ParseCIDR(args[0])
				if err != nil {
					return args, false, fmt.Errorf("bad cidr value for %s", flag)
				}
				mask = netw.Mask
				args = args[1:]
			} else if len(args) < 2 {
				return args, false, fmt.Errorf("missing mask value for %s", flag)
			} else if strings.Contains(args[1], `/`) {
				var netw *net.IPNet
				ip, netw, err = net.ParseCIDR(args[0] + args[1])
				if err != nil {
					return args, false, fmt.Errorf("bad cidr value for %s", flag)
				}
				mask = netw.Mask
				args = args[2:]
			} else {
				ip = net.ParseIP(args[0])
				mask = net.IPMask(net.ParseIP(args[1]).To4())
				args = args[2:]
			}

			if ip == nil {
				return args, false, fmt.Errorf("bad ip value for %s", flag)
			}

			if mask == nil {
				return args, false, fmt.Errorf("bad mask value for %s", flag)
			}
			res[long] = &FlagMapItem{
				Value:     IPandMASK{IP: ip, Mask: mask},
				IsDefault: false,
			}
			return args, true, nil
		})
}

type IPandMASK struct {
	IP   net.IP
	Mask net.IPMask
}
