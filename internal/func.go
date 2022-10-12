/*
Copyright 2022 @mcandeia
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package internal

// Always returns a function that wrap the value
func Always[T any](value T) func() T {
	return func() T {
		return value
	}
}

// WrapF returns a function that waits to be called to apply the parameter.
func WrapF[From any, To any](applier func(From) To, from From) func() To {
	return func() To {
		return applier(from)
	}
}
