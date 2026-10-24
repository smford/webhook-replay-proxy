/**
 * Google Analytics 4 tracking module for GitHub Pages docs.
 * Measurement ID: webhook-replay-proxy
 */

(function () {
  'use strict';

  var MEASUREMENT_ID = 'webhook-replay-proxy';

  function isStandalone() {
    return window.matchMedia('(display-mode: standalone)').matches || window.navigator.standalone === true;
  }

  // Safe wrapper around window.gtag
  function trackEvent(eventName, params) {
    try {
      if (typeof window.gtag === 'function') {
        var eventParams = Object.assign({}, params || {}, {
          pwa_mode: isStandalone() ? 'standalone' : 'browser'
        });
        window.gtag('event', eventName, eventParams);
      }
    } catch (e) {
      // Avoid breaking UI if ad-blocker or network issues occur
      console.warn('Analytics event tracking suppressed:', e);
    }
  }

  function trackPageView(path) {
    trackEvent('page_view', {
      page_path: path || window.location.pathname,
      page_title: document.title,
    });
  }

  function trackAppOpen() {
    trackEvent('app_open', {
      timestamp: new Date().toISOString(),
    });
  }

  // Auto track clicks on install copy buttons, download links & external links
  document.addEventListener('DOMContentLoaded', function () {
    trackPageView();
    trackAppOpen();

    // Copy command tracking
    var copyButtons = document.querySelectorAll('[data-copy]');
    copyButtons.forEach(function (btn) {
      btn.addEventListener('click', function () {
        var command = btn.getAttribute('data-copy');
        trackEvent('copy_command', {
          command_text: command,
        });
      });
    });

    // Release download tracking
    var downloadLinks = document.querySelectorAll('a[href*="/releases/download/"]');
    downloadLinks.forEach(function (link) {
      link.addEventListener('click', function () {
        trackEvent('download_release', {
          link_url: link.href,
        });
      });
    });

    // External link tracking
    var extLinks = document.querySelectorAll('a[target="_blank"]');
    extLinks.forEach(function (link) {
      link.addEventListener('click', function () {
        trackEvent('external_link_click', {
          link_url: link.href,
        });
      });
    });
  });

  // Expose to window
  window.webhookProxyAnalytics = {
    measurementId: MEASUREMENT_ID,
    trackEvent: trackEvent,
    trackPageView: trackPageView,
    trackAppOpen: trackAppOpen,
  };
})();
