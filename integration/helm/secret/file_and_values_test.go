package secret_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/flant/werf/pkg/testing/utils"
)

var _ = Describe("helm secret file/values encrypt/decrypt", func() {
	decryptAndCheckFileOrValues := func(secretType, fileToProcess string, withPipe bool) {
		if withPipe {
			runSucceedCommandWithFileDataOnStdin([]string{"helm", "secret", secretType, "decrypt", "-o", "result"}, fileToProcess)
		} else {
			utils.RunSucceedCommand(
				testDirPath,
				werfBinPath,
				"helm", "secret", secretType, "decrypt", fileToProcess, "-o", "result",
			)
		}

		fileContentsShouldBeEqual("result", "secret")
	}

	var decryptItBody = func(secretType string, withPipe bool) {
		utils.CopyIn(utils.FixturePath(secretType), testDirPath)
		decryptAndCheckFileOrValues(secretType, "encrypted_secret", withPipe)
	}

	var encryptItBody = func(secretType string, withPipe bool) {
		utils.CopyIn(utils.FixturePath(secretType), testDirPath)

		if withPipe {
			runSucceedCommandWithFileDataOnStdin([]string{"helm", "secret", secretType, "encrypt", "-o", "result"}, "secret")
		} else {
			utils.RunSucceedCommand(
				testDirPath,
				werfBinPath,
				"helm", "secret", secretType, "encrypt", "secret", "-o", "result",
			)
		}

		decryptAndCheckFileOrValues(secretType, "result", withPipe)
	}

	var _ = DescribeTable("encryption", encryptItBody,
		Entry("secret file", "file", false),
		Entry("secret file (pipe)", "file", true),
		Entry("secret values", "values", false),
		Entry("secret values (pipe)", "values", true))

	var _ = DescribeTable("decryption", decryptItBody,
		Entry("secret file", "file", false),
		Entry("secret file (pipe)", "file", true),
		Entry("secret values", "values", false),
		Entry("secret values (pipe)", "values", true))
})

func fileContentsShouldBeEqual(path1, path2 string) {
	data1, err := ioutil.ReadFile(filepath.Join(testDirPath, path1))
	Ω(err).ShouldNot(HaveOccurred())

	data2, err := ioutil.ReadFile(filepath.Join(testDirPath, path2))
	Ω(err).ShouldNot(HaveOccurred())

	data1 = bytes.ReplaceAll(data1, []byte(utils.LineBreak), []byte("\n"))
	data2 = bytes.ReplaceAll(data2, []byte(utils.LineBreak), []byte("\n"))

	_, _ = fmt.Fprintf(GinkgoWriter, "=== %s ===\n", filepath.Join(testDirPath, path1))
	_, _ = fmt.Fprintf(GinkgoWriter, string(data1))
	_, _ = fmt.Fprintf(GinkgoWriter, "=== %s ===\n", filepath.Join(testDirPath, path1))

	_, _ = fmt.Fprintf(GinkgoWriter, "=== %s ===\n", filepath.Join(testDirPath, path2))
	_, _ = fmt.Fprintf(GinkgoWriter, string(data2))
	_, _ = fmt.Fprintf(GinkgoWriter, "=== %s ===\n", filepath.Join(testDirPath, path2))

	Ω(bytes.Equal(data1, data2)).Should(BeTrue())
}

func runSucceedCommandWithFileDataOnStdin(werfArgs []string, secretFileName string) {
	data, err := ioutil.ReadFile(filepath.Join(testDirPath, secretFileName))

	Ω(err).ShouldNot(HaveOccurred())

	_, _ = utils.RunCommandWithOptions(
		testDirPath,
		werfBinPath,
		werfArgs,
		utils.RunCommandOptions{
			ToStdin:       string(data),
			ShouldSucceed: true,
		},
	)
}
