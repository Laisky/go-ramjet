package gitlab

import (
	"reflect"
	"testing"

	"github.com/Laisky/testify/require"
)

func Test_parseGitFileReq(t *testing.T) {
	type args struct {
		file string
	}
	tests := []struct {
		name string
		args args
		want *GitFileURL
	}{

		{"0", args{"https://git.basebit.me/xss/doc/-/blob/master/.gitlab-ci.yml"}, &GitFileURL{"xss%2Fdoc", "master", ".gitlab-ci.yml", 0, 0}},
		{"1", args{"https://git.basebit.me/xss/doc/-/blob/master/.gitlab-ci.yml#L1"}, &GitFileURL{"xss%2Fdoc", "master", ".gitlab-ci.yml", 1, 0}},
		{"2", args{"https://git.basebit.me/xss/doc/-/blob/master/.gitlab-ci.yml#L1-2"}, &GitFileURL{"xss%2Fdoc", "master", ".gitlab-ci.yml", 1, 2}},
		{"3", args{"https://git.basebit.me/xss/doc/-/blob/9e7ae1e7ce7cb4028ee00c0fd45436d46db75159/.gitlab-ci.yml"}, &GitFileURL{"xss%2Fdoc", "9e7ae1e7ce7cb4028ee00c0fd45436d46db75159", ".gitlab-ci.yml", 0, 0}},
		{"4", args{"https://git.basebit.me/xss/doc/-/blob/9e7ae1e7ce7cb4028ee00c0fd45436d46db75159/.gitlab-ci.yml#L1"}, &GitFileURL{"xss%2Fdoc", "9e7ae1e7ce7cb4028ee00c0fd45436d46db75159", ".gitlab-ci.yml", 1, 0}},
		{"5", args{"https://git.basebit.me/xss/doc/-/blob/9e7ae1e7ce7cb4028ee00c0fd45436d46db75159/.gitlab-ci.yml#L1-2"}, &GitFileURL{"xss%2Fdoc", "9e7ae1e7ce7cb4028ee00c0fd45436d46db75159", ".gitlab-ci.yml", 1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGitFileReq(tt.args.file)
			require.NoError(t, err)

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseGitFileReq() = %v, want %v", got, tt.want)
			}
		})
	}
}
