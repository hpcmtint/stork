package dumper

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
	"isc.org/stork/server/dumper/dumps"
	storkutil "isc.org/stork/util"
)

// Test that the saver is properly constructed.
func TestConstructSaver(t *testing.T) {
	// Act
	saver := newTarbalSaver(
		json.Marshal,
		func(dump dumps.Dump, artifact dumps.Artifact) string { return "" },
	)

	// Assert
	require.NotNil(t, saver)
}

// Test that the saver creates the archive from the empty data.
func TestSaverSaveEmptyDumpList(t *testing.T) {
	// Arrange
	saver := newTarbalSaver(
		json.Marshal,
		func(dump dumps.Dump, artifact dumps.Artifact) string { return "" },
	)
	var buffer bytes.Buffer

	// Act
	err := saver.Save(&buffer, []dumps.Dump{})

	// Assert
	require.NoError(t, err)
	require.Len(t, buffer.Bytes(), 32)
}

// Test that the saver creates the archive from the non-empty data.
func TestSaverSaveFilledDumpList(t *testing.T) {
	// Arrange
	saver := newTarbalSaver(
		json.Marshal,
		func(dump dumps.Dump, artifact dumps.Artifact) string {
			return dump.Name() + artifact.Name()
		},
	)
	var buffer bytes.Buffer

	// Act
	dumps := []dumps.Dump{
		dumps.NewBasicDump(
			"foo",
			dumps.NewBasicStructArtifact("bar", 42),
		),
		dumps.NewBasicDump(
			"baz",
			dumps.NewBasicBinaryArtifact("biz", []byte{42, 24}),
			dumps.NewBasicStructArtifact("boz", "buz"),
		),
	}
	err := saver.Save(&buffer, dumps)

	// Assert
	require.NoError(t, err)
	require.EqualValues(t, buffer.Len(), 145)
}

// Test that the output tarball has proper content.
func TestSavedTarball(t *testing.T) {
	// Arrange
	saver := newTarbalSaver(
		json.Marshal,
		func(dump dumps.Dump, artifact dumps.Artifact) string {
			return dump.Name() + artifact.Name()
		},
	)
	var buffer bytes.Buffer

	dumps := []dumps.Dump{
		dumps.NewBasicDump(
			"foo",
			dumps.NewBasicStructArtifact("bar", 42),
		),
		dumps.NewBasicDump(
			"baz",
			dumps.NewBasicBinaryArtifact("biz", []byte{42, 24}),
			dumps.NewBasicStructArtifact("boz", "buz"),
		),
	}
	_ = saver.Save(&buffer, dumps)
	bufferBytes := buffer.Bytes()

	expectedFooBarContent, _ := json.Marshal(42)
	expectedBazBozContent, _ := json.Marshal("buz")

	// Act
	filenames, listErr := storkutil.ListFilesInTarball(bytes.NewReader(bufferBytes))
	fooBarContent, fooBarErr := storkutil.SearchFileInTarball(bytes.NewReader(bufferBytes), "foobar")
	bazBozContent, bazBozErr := storkutil.SearchFileInTarball(bytes.NewReader(bufferBytes), "bazboz")

	// Assert
	require.NoError(t, listErr)
	require.NoError(t, fooBarErr)
	require.NoError(t, bazBozErr)

	require.Len(t, filenames, 3)

	require.EqualValues(t, expectedFooBarContent, fooBarContent)
	require.EqualValues(t, expectedBazBozContent, bazBozContent)
}

// Test if the tarbal is properly saved to file.
func TestSavedTarballToFile(t *testing.T) {
	// Arrange
	saver := newTarbalSaver(
		json.Marshal,
		func(dump dumps.Dump, artifact dumps.Artifact) string {
			return dump.Name() + artifact.Name()
		},
	)
	file, _ := ioutil.TempFile("", "*")
	bufferWriter := bufio.NewWriter(file)

	// Act
	err := saver.Save(bufferWriter, []dumps.Dump{})
	_ = bufferWriter.Flush()
	stat, _ := file.Stat()
	position, _ := file.Seek(0, io.SeekCurrent)

	// Assert
	require.NoError(t, err)
	require.NotZero(t, stat.Size())
	require.NotZero(t, position)
}