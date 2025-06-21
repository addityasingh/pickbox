package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

func main() {
	// Set up logging
	logrus.SetLevel(logrus.InfoLevel)
	logger := logrus.New()

	// Create directories for 3 nodes
	nodes := []string{"node1", "node2", "node3"}
	baseDir := "data"

	// Clean up existing data
	os.RemoveAll(baseDir)

	// Create node directories
	for _, node := range nodes {
		nodeDir := filepath.Join(baseDir, node)
		if err := os.MkdirAll(nodeDir, 0755); err != nil {
			logger.WithError(err).Fatalf("Failed to create directory for %s", node)
		}
		logger.Infof("Created directory for %s", node)
	}

	// Simulate leader election - node1 becomes leader
	leader := "node1"
	logger.Infof("%s elected as leader", leader)

	// Create a test file on the leader
	testFile := "test.txt"
	testContent := "Hello, this is a test file for replication!"

	leaderFilePath := filepath.Join(baseDir, leader, testFile)
	if err := os.WriteFile(leaderFilePath, []byte(testContent), 0644); err != nil {
		logger.WithError(err).Fatal("Failed to write test file")
	}
	logger.Infof("Created test file on leader %s: %s", leader, testFile)

	// Simulate replication - copy file to followers
	for _, node := range nodes {
		if node == leader {
			continue // Skip leader
		}

		followerFilePath := filepath.Join(baseDir, node, testFile)
		if err := os.WriteFile(followerFilePath, []byte(testContent), 0644); err != nil {
			logger.WithError(err).Errorf("Failed to replicate file to %s", node)
			continue
		}
		logger.Infof("Replicated file to follower %s", node)
	}

	// Wait a moment
	time.Sleep(1 * time.Second)

	// Verify replication by checking all nodes have the same content
	logger.Info("Verifying replication...")
	for _, node := range nodes {
		nodeFilePath := filepath.Join(baseDir, node, testFile)
		content, err := os.ReadFile(nodeFilePath)
		if err != nil {
			logger.WithError(err).Errorf("Failed to read file from %s", node)
			continue
		}

		if string(content) == testContent {
			logger.Infof("✓ %s: File content matches", node)
		} else {
			logger.Errorf("✗ %s: File content mismatch", node)
		}
	}

	// Show directory contents
	logger.Info("Directory contents:")
	for _, node := range nodes {
		nodeDir := filepath.Join(baseDir, node)
		logger.Infof("Contents of %s:", node)

		err := filepath.WalkDir(nodeDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				relPath, _ := filepath.Rel(nodeDir, path)
				info, _ := d.Info()
				logger.Infof("  %s (%d bytes)", relPath, info.Size())
			}
			return nil
		})

		if err != nil {
			logger.WithError(err).Errorf("Failed to walk directory %s", node)
		}
	}

	// Simulate editing a file on the leader
	logger.Info("Simulating file edit on leader...")
	updatedContent := testContent + "\nThis line was added by the leader!"

	if err := os.WriteFile(leaderFilePath, []byte(updatedContent), 0644); err != nil {
		logger.WithError(err).Fatal("Failed to update test file")
	}
	logger.Infof("Updated file on leader %s", leader)

	// Replicate the update
	time.Sleep(500 * time.Millisecond)
	for _, node := range nodes {
		if node == leader {
			continue
		}

		followerFilePath := filepath.Join(baseDir, node, testFile)
		if err := os.WriteFile(followerFilePath, []byte(updatedContent), 0644); err != nil {
			logger.WithError(err).Errorf("Failed to replicate update to %s", node)
			continue
		}
		logger.Infof("Replicated update to follower %s", node)
	}

	// Final verification
	logger.Info("Final verification after update:")
	for _, node := range nodes {
		nodeFilePath := filepath.Join(baseDir, node, testFile)
		content, err := os.ReadFile(nodeFilePath)
		if err != nil {
			logger.WithError(err).Errorf("Failed to read updated file from %s", node)
			continue
		}

		if string(content) == updatedContent {
			logger.Infof("✓ %s: Updated content matches", node)
		} else {
			logger.Errorf("✗ %s: Updated content mismatch", node)
		}
	}

	logger.Info("Demo completed successfully!")
	fmt.Println("\nYou can now check the files in the data/ directory:")
	fmt.Println("- data/node1/test.txt")
	fmt.Println("- data/node2/test.txt")
	fmt.Println("- data/node3/test.txt")
	fmt.Println("\nAll files should have identical content showing replication worked!")
}
