package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sampson/npm-autodiscovery/internal/api"
	"github.com/sampson/npm-autodiscovery/internal/config"
	"github.com/sampson/npm-autodiscovery/internal/docker"
	"github.com/sampson/npm-autodiscovery/internal/lxd"
	"github.com/sampson/npm-autodiscovery/internal/npm"
	"github.com/sampson/npm-autodiscovery/internal/pve"
	"github.com/sampson/npm-autodiscovery/internal/syncer"
	"github.com/sampson/npm-autodiscovery/web"
)

const banner = `
=============================================================
  _   _ _____  __  __      _         _          ____  _      
 | \ | |  __ \|  \/  |    / \  _   _| |_ ___   |  _ \(_)___  
 |  \| | |__) | |\/| |   / _ \| | | | __/ _ \  | | | | / __| 
 | |\  |  ___/| |  | |  / ___ \ |_| | || (_) | | |_| | \__ \ 
 |_| \_|_|    |_|  |_| /_/   \_\__,_|\__\___/  |____/|_|___/ 
                                                             
     Automated Ingress & Discovery Engine (v2.0.0)       
   Docker • Proxmox LXC • LXD/Incus • Layer 4 Streams • IaC  
=============================================================
`

func main() {
	fmt.Print(banner)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("[info] Starting NPM Auto-Discovery v2.0.0 (Production Release)")

	// 1. Load configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("[fatal] Configuration error: %v", err)
	}

	log.Printf("[info] Loaded configuration: NPM_URL=%s, NPM_USER=%s, Socket=%s, PollInterval=%s, Port=%d, PVE_ENABLED=%v, LXD_ENABLED=%v",
		cfg.NPMURL, cfg.NPMUser, cfg.DockerSocket, cfg.PollInterval, cfg.Port, cfg.PVEEnabled, cfg.LXDEnabled)

	// 2. Initialize Docker Client (optional in standalone hypervisor mode)
	dockerClient, err := docker.NewClient(cfg.DockerSocket, cfg.DockerTimeout)
	if err != nil {
		if !cfg.PVEEnabled && !cfg.LXDEnabled && cfg.RoutesFile == "" && cfg.RoutesDir == "" {
			log.Fatalf("[fatal] Failed to initialize Docker client: %v", err)
		}
		log.Printf("[warn] Docker socket unavailable (%v). Continuing in standalone hypervisor/IaC mode.", err)
		dockerClient = nil
	}

	// 3. Initialize NPM Client
	npmClient := npm.NewClient(cfg.NPMURL, cfg.NPMUser, cfg.NPMPass, cfg.NPMTimeout)

	// 4. Initialize Proxmox VE Provider (Phase 3.1)
	var pveClient *pve.Client
	if cfg.PVEEnabled {
		pveClient, err = pve.NewClient(
			cfg.PVEURL,
			cfg.PVETokenID,
			cfg.PVETokenSecret,
			cfg.PVENode,
			cfg.PVEVerifySSL,
			10*time.Second,
			cfg.PVEPreferredInterface,
			cfg.PVEAllowedSubnets,
		)
		if err != nil {
			log.Printf("[warn] Proxmox VE client initialization error: %v", err)
		} else {
			log.Printf("[info] Initialized Proxmox VE provider: URL=%s, Node=%s, PreferredInterface=%s",
				cfg.PVEURL, cfg.PVENode, cfg.PVEPreferredInterface)
		}
	}

	// 5. Initialize Canonical LXD / Incus Provider (Phase 3.2)
	var lxdClient *lxd.Client
	if cfg.LXDEnabled {
		lxdClient, err = lxd.NewClient(
			cfg.LXDSocket,
			10*time.Second,
			cfg.LXDPreferredInterface,
			cfg.LXDAllowedSubnets,
		)
		if err != nil {
			log.Printf("[warn] Canonical LXD / Incus client initialization error: %v", err)
		} else {
			log.Printf("[info] Initialized LXD/Incus provider: Socket=%s, PreferredInterface=%s",
				cfg.LXDSocket, cfg.LXDPreferredInterface)
		}
	}

	// 6. Load embedded Web UI
	webFS, err := web.GetFS()
	if err != nil {
		log.Printf("[warn] Embedded web assets warning: %v", err)
	}

	// 7. Initialize Syncer
	syncerEngine := syncer.NewSyncerWithProviders(cfg, dockerClient, npmClient, pveClient, lxdClient)

	// 8. Initialize HTTP Server
	apiServer := api.NewServer(cfg, syncerEngine, webFS)

	// 7. Setup Context & Graceful Shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Start Syncer Engine
	go syncerEngine.Start(ctx)

	// Start API & Dashboard HTTP Server
	serverErrChan := make(chan error, 1)
	go func() {
		if err := apiServer.Start(); err != nil && err != http.ErrServerClosed {
			serverErrChan <- err
		}
	}()

	log.Printf("[info] NPM Auto-Discovery engine running. Dashboard ready on http://localhost:%d", cfg.Port)

	// Wait for shutdown signal or fatal server error
	select {
	case sig := <-sigChan:
		log.Printf("[info] Received signal %v. Initiating graceful shutdown...", sig)
	case err := <-serverErrChan:
		log.Fatalf("[fatal] HTTP server error: %v", err)
	}

	// Graceful shutdown sequence
	cancel() // Stop syncer background routines
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := apiServer.Stop(shutdownCtx); err != nil {
		log.Printf("[warn] HTTP server shutdown error: %v", err)
	}

	log.Println("[info] NPM Auto-Discovery shut down cleanly.")
}
