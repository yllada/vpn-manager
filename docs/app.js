/**
 * VPN Manager Landing Page JavaScript
 * 
 * Modules:
 * 1. Theme Toggle
 * 2. Scroll Reveal
 * 3. Copy Install Command
 * 4. Dynamic Version & Install Commands
 * 5. GitHub Stats
 * 6. Screenshot Carousel
 * 7. Typing Effect
 * 8. Parallax Mouse Effect
 * 9. Floating CTA
 * 10. Header Scroll Effect
 * 11. Contributors
 */

(function() {
  'use strict';

  const REPO = 'yllada/vpn-manager';

  // ===== 1. THEME TOGGLE =====
  function initThemeToggle() {
    const toggle = document.getElementById('theme-toggle');
    if (!toggle) return;

    const html = document.documentElement;
    const metaThemeColor = document.getElementById('theme-color-meta');
    
    function setTheme(isDark) {
      if (isDark) {
        html.classList.add('dark');
        if (metaThemeColor) metaThemeColor.content = '#0f172a';
        toggle.setAttribute('aria-checked', 'true');
      } else {
        html.classList.remove('dark');
        if (metaThemeColor) metaThemeColor.content = '#f8fafc';
        toggle.setAttribute('aria-checked', 'false');
      }
      localStorage.setItem('vpn-manager-theme', isDark ? 'dark' : 'light');
    }
    
    toggle.addEventListener('click', () => {
      const isDark = !html.classList.contains('dark');
      setTheme(isDark);
    });
    
    // Handle keyboard
    toggle.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        toggle.click();
      }
    });
    
    // Listen for system theme changes
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
      if (!localStorage.getItem('vpn-manager-theme')) {
        setTheme(e.matches);
      }
    });
    
    // Set initial aria-checked state
    toggle.setAttribute('aria-checked', html.classList.contains('dark') ? 'true' : 'false');
  }

  // ===== 2. SCROLL REVEAL =====
  function initScrollReveal() {
    const observerOptions = {
      root: null,
      rootMargin: '0px',
      threshold: 0.1
    };
    
    const observer = new IntersectionObserver((entries) => {
      entries.forEach(entry => {
        if (entry.isIntersecting) {
          entry.target.classList.add('visible');
        }
      });
    }, observerOptions);
    
    // Observe all reveal elements
    document.querySelectorAll('.reveal, .reveal-left, .reveal-right, .reveal-scale, .stagger-children').forEach(el => {
      observer.observe(el);
    });
  }

  // ===== 3. COPY INSTALL COMMAND =====
  window.copyInstallCommand = function(panelId) {
    const panel = document.getElementById(panelId);
    if (!panel) return;
    
    const codeBlock = panel.querySelector('[data-install-cmd]');
    if (!codeBlock) return;
    
    const text = codeBlock.innerText;
    
    navigator.clipboard.writeText(text).then(() => {
      const btn = panel.querySelector('.copy-btn');
      if (btn) {
        const originalSvg = btn.innerHTML;
        btn.innerHTML = `<svg class="w-5 h-5 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
        </svg>`;
        
        setTimeout(() => {
          btn.innerHTML = originalSvg;
        }, 2000);
      }
    }).catch(err => {
      console.error('Failed to copy:', err);
    });
  };

  // ===== 4. DYNAMIC VERSION & INSTALL COMMANDS =====
  function initDynamicVersion() {
    const CACHE_KEY = 'vpn-manager-release';
    const CACHE_TTL = 3600000; // 1 hour

    async function getLatestRelease() {
      const cached = localStorage.getItem(CACHE_KEY);
      if (cached) {
        const { data, timestamp } = JSON.parse(cached);
        if (Date.now() - timestamp < CACHE_TTL) {
          return data;
        }
      }

      try {
        const response = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`);
        if (!response.ok) throw new Error('API error');
        const data = await response.json();
        
        const releaseData = {
          tag_name: data.tag_name,
          html_url: data.html_url,
          assets: data.assets.map(a => ({
            name: a.name,
            browser_download_url: a.browser_download_url
          }))
        };

        localStorage.setItem(CACHE_KEY, JSON.stringify({
          data: releaseData,
          timestamp: Date.now()
        }));
        
        return releaseData;
      } catch (error) {
        console.warn('Failed to fetch release:', error);
        return null;
      }
    }

    function findAsset(assets, pattern) {
      return assets.find(a => a.name.includes(pattern));
    }

    function generateCommands(assets, version) {
      const debAsset = findAsset(assets, '.deb');
      const rpmAsset = findAsset(assets, '.rpm');
      const tarAsset = findAsset(assets, '.tar.gz');
      
      return {
        ubuntu: debAsset 
          ? `<span class="text-slate-400 dark:text-slate-500"># Download and install VPN Manager ${version}</span>
<span class="text-gnome">wget</span> ${debAsset.browser_download_url}
<span class="text-gnome">sudo</span> dpkg -i ${debAsset.name}

<span class="text-slate-400 dark:text-slate-500"># Run</span>
vpn-manager`
          : generateTarballCommand(tarAsset, version),
          
        fedora: rpmAsset
          ? `<span class="text-slate-400 dark:text-slate-500"># Download and install VPN Manager ${version}</span>
<span class="text-gnome">wget</span> ${rpmAsset.browser_download_url}
<span class="text-gnome">sudo</span> dnf install ./${rpmAsset.name}

<span class="text-slate-400 dark:text-slate-500"># Run</span>
vpn-manager`
          : generateTarballCommand(tarAsset, version),
          
        arch: generateTarballCommand(tarAsset, version)
      };
    }
    
    function generateTarballCommand(tarAsset, version) {
      if (!tarAsset) return '<span class="text-slate-400 dark:text-slate-500"># No release available, build from source</span>';
      return `<span class="text-slate-400 dark:text-slate-500"># Download and install VPN Manager ${version}</span>
<span class="text-gnome">wget</span> ${tarAsset.browser_download_url}
<span class="text-gnome">tar</span> xzf ${tarAsset.name}
<span class="text-gnome">sudo</span> mv vpn-manager /usr/local/bin/

<span class="text-slate-400 dark:text-slate-500"># Run</span>
vpn-manager`;
    }

    async function updatePage() {
      const release = await getLatestRelease();
      
      const versionBadge = document.getElementById('version-badge');
      if (versionBadge) {
        versionBadge.textContent = release ? release.tag_name : 'Latest';
      }

      if (release && release.assets.length > 0) {
        const commands = generateCommands(release.assets, release.tag_name);
        
        const ubuntuPanel = document.querySelector('#panel-ubuntu [data-install-cmd]');
        const fedoraPanel = document.querySelector('#panel-fedora [data-install-cmd]');
        const archPanel = document.querySelector('#panel-arch [data-install-cmd]');
        
        if (ubuntuPanel) ubuntuPanel.innerHTML = commands.ubuntu;
        if (fedoraPanel) fedoraPanel.innerHTML = commands.fedora;
        if (archPanel) archPanel.innerHTML = commands.arch;
      }
    }

    updatePage();
  }

  // ===== 5. GITHUB STATS =====
  function initGitHubStats() {
    const STATS_CACHE_KEY = 'vpn-manager-stats';
    const STATS_CACHE_TTL = 300000; // 5 minutes
    
    function formatNumber(num) {
      if (num >= 1000000) {
        return (num / 1000000).toFixed(1).replace(/\.0$/, '') + 'M';
      }
      if (num >= 1000) {
        return (num / 1000).toFixed(1).replace(/\.0$/, '') + 'K';
      }
      return num.toString();
    }
    
    function formatRelativeTime(dateString) {
      const date = new Date(dateString);
      const now = new Date();
      const diffMs = now - date;
      const diffMins = Math.floor(diffMs / 60000);
      const diffHours = Math.floor(diffMs / 3600000);
      const diffDays = Math.floor(diffMs / 86400000);
      
      if (diffMins < 60) return `${diffMins}m ago`;
      if (diffHours < 24) return `${diffHours}h ago`;
      if (diffDays < 30) return `${diffDays}d ago`;
      return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }
    
    function animateValue(element, start, end, duration) {
      const startTime = performance.now();
      const isFormatted = end >= 1000;
      
      function update(currentTime) {
        const elapsed = currentTime - startTime;
        const progress = Math.min(elapsed / duration, 1);
        
        // Ease out cubic
        const eased = 1 - Math.pow(1 - progress, 3);
        const current = Math.floor(start + (end - start) * eased);
        
        element.textContent = isFormatted ? formatNumber(current) : current;
        
        if (progress < 1) {
          requestAnimationFrame(update);
        } else {
          element.textContent = formatNumber(end);
        }
      }
      
      requestAnimationFrame(update);
    }
    
    async function fetchGitHubStats() {
      // Check cache first
      const cached = localStorage.getItem(STATS_CACHE_KEY);
      if (cached) {
        const { data, timestamp } = JSON.parse(cached);
        if (Date.now() - timestamp < STATS_CACHE_TTL) {
          return data;
        }
      }
      
      try {
        // Fetch repo info, releases, and commits in parallel
        const [repoResponse, releasesResponse, commitsResponse] = await Promise.all([
          fetch(`https://api.github.com/repos/${REPO}`),
          fetch(`https://api.github.com/repos/${REPO}/releases`),
          fetch(`https://api.github.com/repos/${REPO}/commits?per_page=1`)
        ]);
        
        if (!repoResponse.ok) throw new Error('API error');
        
        const repo = await repoResponse.json();
        const releases = releasesResponse.ok ? await releasesResponse.json() : [];
        const commits = commitsResponse.ok ? await commitsResponse.json() : [];
        
        // Calculate total downloads across all releases
        let totalDownloads = 0;
        releases.forEach(release => {
          release.assets.forEach(asset => {
            totalDownloads += asset.download_count;
          });
        });
        
        const stats = {
          stars: repo.stargazers_count,
          forks: repo.forks_count,
          openIssues: repo.open_issues_count,
          downloads: totalDownloads,
          releases: releases.length,
          license: repo.license?.spdx_id || 'MIT',
          lastCommit: commits[0]?.commit?.committer?.date || null
        };
        
        // Cache the results
        localStorage.setItem(STATS_CACHE_KEY, JSON.stringify({
          data: stats,
          timestamp: Date.now()
        }));
        
        return stats;
      } catch (error) {
        console.warn('Failed to fetch GitHub stats:', error);
        return null;
      }
    }
    
    async function updateStats() {
      const stats = await fetchGitHubStats();
      if (!stats) return;
      
      // Animate the main stats
      const starsEl = document.getElementById('stat-stars');
      const forksEl = document.getElementById('stat-forks');
      const downloadsEl = document.getElementById('stat-downloads');
      const releasesEl = document.getElementById('stat-releases');
      const issuesEl = document.getElementById('stat-issues');
      const commitEl = document.getElementById('stat-commit');
      const licenseEl = document.getElementById('stat-license');
      
      if (starsEl && stats.stars !== undefined) {
        animateValue(starsEl, 0, stats.stars, 1500);
      }
      if (forksEl && stats.forks !== undefined) {
        animateValue(forksEl, 0, stats.forks, 1500);
      }
      if (downloadsEl && stats.downloads !== undefined) {
        animateValue(downloadsEl, 0, stats.downloads, 1500);
      }
      if (releasesEl && stats.releases !== undefined) {
        animateValue(releasesEl, 0, stats.releases, 1500);
      }
      if (issuesEl && stats.openIssues !== undefined) {
        issuesEl.textContent = stats.openIssues;
      }
      if (commitEl && stats.lastCommit) {
        commitEl.textContent = formatRelativeTime(stats.lastCommit);
      }
      if (licenseEl && stats.license) {
        licenseEl.textContent = stats.license;
      }
    }

    updateStats();
  }

  // ===== 6. SCREENSHOT CAROUSEL =====
  function initScreenshotCarousel() {
    const tabs = document.querySelectorAll('.screenshot-tab');
    const images = document.querySelectorAll('.screenshot-img');
    const container = document.querySelector('.screenshot-container');
    
    if (!tabs.length || !images.length) return;
    
    let currentIndex = 0;
    let autoRotateInterval = null;
    let isPaused = false;
    const AUTO_ROTATE_DELAY = 5000; // 5 seconds
    
    function showScreenshot(screenshotName) {
      // Update tabs
      tabs.forEach(tab => {
        if (tab.dataset.screenshot === screenshotName) {
          tab.classList.add('active');
        } else {
          tab.classList.remove('active');
        }
      });
      
      // Update images
      images.forEach(img => {
        if (img.dataset.screenshot === screenshotName) {
          img.classList.add('active');
        } else {
          img.classList.remove('active');
        }
      });
      
      // Update current index
      const tabsArray = Array.from(tabs);
      currentIndex = tabsArray.findIndex(tab => tab.dataset.screenshot === screenshotName);
    }
    
    function nextScreenshot() {
      const tabsArray = Array.from(tabs);
      currentIndex = (currentIndex + 1) % tabsArray.length;
      const nextTab = tabsArray[currentIndex];
      showScreenshot(nextTab.dataset.screenshot);
    }
    
    function startAutoRotate() {
      if (autoRotateInterval) clearInterval(autoRotateInterval);
      autoRotateInterval = setInterval(() => {
        if (!isPaused) {
          nextScreenshot();
        }
      }, AUTO_ROTATE_DELAY);
    }
    
    function stopAutoRotate() {
      if (autoRotateInterval) {
        clearInterval(autoRotateInterval);
        autoRotateInterval = null;
      }
    }
    
    // Tab click handlers
    tabs.forEach(tab => {
      tab.addEventListener('click', () => {
        showScreenshot(tab.dataset.screenshot);
        // Reset auto-rotate timer on manual interaction
        startAutoRotate();
      });
      
      // Keyboard accessibility
      tab.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          tab.click();
        }
      });
    });
    
    // Pause on hover
    if (container) {
      container.addEventListener('mouseenter', () => {
        isPaused = true;
      });
      
      container.addEventListener('mouseleave', () => {
        isPaused = false;
      });
    }
    
    // Start auto-rotation
    startAutoRotate();
    
    // Pause when page is not visible
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) {
        stopAutoRotate();
      } else {
        startAutoRotate();
      }
    });
  }

  // ===== 7. TYPING EFFECT =====
  function initTypingEffect() {
    const textElement = document.getElementById('typing-text');
    const cursorElement = document.getElementById('typing-cursor');
    
    if (!textElement) return;
    
    const words = ['OpenVPN', 'WireGuard', 'Tailscale'];
    let wordIndex = 0;
    let charIndex = 0;
    let isDeleting = false;
    let isPaused = false;
    
    const TYPING_SPEED = 100;
    const DELETING_SPEED = 60;
    const PAUSE_AFTER_WORD = 2000;
    const PAUSE_BEFORE_TYPING = 500;
    
    function type() {
      const currentWord = words[wordIndex];
      
      if (isPaused) {
        setTimeout(type, 100);
        return;
      }
      
      if (isDeleting) {
        // Deleting characters
        charIndex--;
        textElement.textContent = currentWord.substring(0, charIndex);
        
        if (charIndex === 0) {
          isDeleting = false;
          wordIndex = (wordIndex + 1) % words.length;
          setTimeout(type, PAUSE_BEFORE_TYPING);
        } else {
          setTimeout(type, DELETING_SPEED);
        }
      } else {
        // Typing characters
        charIndex++;
        textElement.textContent = currentWord.substring(0, charIndex);
        
        if (charIndex === currentWord.length) {
          isDeleting = true;
          setTimeout(type, PAUSE_AFTER_WORD);
        } else {
          setTimeout(type, TYPING_SPEED);
        }
      }
    }
    
    // Start typing after initial animation delay
    setTimeout(() => {
      type();
    }, 1500);
    
    // Pause when page is not visible
    document.addEventListener('visibilitychange', () => {
      isPaused = document.hidden;
    });
  }

  // ===== 8. PARALLAX MOUSE EFFECT =====
  function initParallax() {
    const blobs = document.querySelectorAll('.blob');
    if (!blobs.length) return;

    // Only run parallax on desktop with no reduced motion preference
    if (!window.matchMedia('(min-width: 768px)').matches || 
        window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      return;
    }

    let mouseX = 0, mouseY = 0;
    let currentX = 0, currentY = 0;
    
    document.addEventListener('mousemove', (e) => {
      mouseX = (e.clientX / window.innerWidth - 0.5) * 30;
      mouseY = (e.clientY / window.innerHeight - 0.5) * 30;
    });
    
    function animate() {
      currentX += (mouseX - currentX) * 0.05;
      currentY += (mouseY - currentY) * 0.05;
      
      blobs.forEach((blob, i) => {
        const factor = (i + 1) * 0.5;
        blob.style.transform = `translate(${currentX * factor}px, ${currentY * factor}px)`;
      });
      
      requestAnimationFrame(animate);
    }
    
    animate();
  }

  // ===== 9. FLOATING CTA =====
  function initFloatingCTA() {
    const floatingCTA = document.getElementById('floating-cta');
    const heroSection = document.querySelector('#hero-heading');
    
    if (!floatingCTA) return;
    
    let ticking = false;
    
    function checkScroll() {
      // Show CTA when user scrolls past the hero section
      const scrollY = window.scrollY;
      const showThreshold = window.innerHeight * 0.8;
      
      if (scrollY > showThreshold) {
        floatingCTA.classList.add('visible');
        floatingCTA.setAttribute('aria-hidden', 'false');
      } else {
        floatingCTA.classList.remove('visible');
        floatingCTA.setAttribute('aria-hidden', 'true');
      }
      
      ticking = false;
    }
    
    window.addEventListener('scroll', () => {
      if (!ticking) {
        requestAnimationFrame(checkScroll);
        ticking = true;
      }
    }, { passive: true });
    
    // Initial check
    checkScroll();
  }

  // ===== 10. HEADER SCROLL EFFECT =====
  function initHeaderScroll() {
    const header = document.querySelector('header');
    if (!header) return;
    
    let ticking = false;
    
    function checkScroll() {
      if (window.scrollY > 50) {
        header.classList.add('scrolled');
      } else {
        header.classList.remove('scrolled');
      }
      ticking = false;
    }
    
    window.addEventListener('scroll', () => {
      if (!ticking) {
        requestAnimationFrame(checkScroll);
        ticking = true;
      }
    }, { passive: true });
    
    // Initial check
    checkScroll();
  }

  // ===== 11. CONTRIBUTORS =====
  function initContributors() {
    const grid = document.getElementById('contributors-grid');
    if (!grid) return;

    const CONTRIBUTORS_CACHE_KEY = 'vpn-manager-contributors';
    const CONTRIBUTORS_CACHE_TTL = 1000 * 60 * 60; // 1 hour

    async function fetchContributors() {
      // Check cache first
      const cached = localStorage.getItem(CONTRIBUTORS_CACHE_KEY);
      if (cached) {
        const { data, timestamp } = JSON.parse(cached);
        if (Date.now() - timestamp < CONTRIBUTORS_CACHE_TTL) {
          return data;
        }
      }

      try {
        const response = await fetch(`https://api.github.com/repos/${REPO}/contributors?per_page=20`);
        if (!response.ok) throw new Error('API error');
        
        const contributors = await response.json();
        
        // Cache the results
        localStorage.setItem(CONTRIBUTORS_CACHE_KEY, JSON.stringify({
          data: contributors,
          timestamp: Date.now()
        }));
        
        return contributors;
      } catch (error) {
        console.warn('Failed to fetch contributors:', error);
        return null;
      }
    }

    async function renderContributors() {
      const contributors = await fetchContributors();
      
      if (!contributors || contributors.length === 0) {
        grid.innerHTML = '<p class="text-slate-400 dark:text-slate-500 text-sm">Could not load contributors</p>';
        return;
      }

      grid.innerHTML = contributors.map(contributor => `
        <a href="${contributor.html_url}" 
           target="_blank" 
           rel="noopener noreferrer" 
           class="group relative" 
           title="${contributor.login} (${contributor.contributions} contributions)">
          <div class="relative">
            <img src="${contributor.avatar_url}" 
                 alt="${contributor.login}" 
                 class="w-14 h-14 sm:w-16 sm:h-16 rounded-full border-2 border-slate-200 dark:border-slate-700 group-hover:border-gnome transition-all duration-300 group-hover:scale-110 group-hover:shadow-lg group-hover:shadow-gnome/20"
                 loading="lazy">
            <div class="absolute -bottom-1 -right-1 bg-gnome text-white text-xs font-bold rounded-full w-5 h-5 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity duration-300">
              ${contributor.contributions}
            </div>
          </div>
          <p class="text-xs text-center mt-2 text-slate-600 dark:text-slate-400 group-hover:text-gnome transition-colors truncate max-w-[64px] sm:max-w-[72px]">
            ${contributor.login}
          </p>
        </a>
      `).join('');
    }

    renderContributors();
  }

  // ===== INITIALIZATION =====
  function init() {
    initThemeToggle();
    initScrollReveal();
    initDynamicVersion();
    initGitHubStats();
    initScreenshotCarousel();
    initTypingEffect();
    initParallax();
    initFloatingCTA();
    initHeaderScroll();
    initContributors();
  }

  // Run on DOM ready
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
