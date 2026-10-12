import React, { useState, useEffect, useCallback } from 'react';
import {
  NavView,
  ConnectionStatus,
  HealthResult,
  ProjectOverview,
  TimelineItem,
  ResumptionReport,
} from './types';
import { Sidebar } from './components/Sidebar';
import { Header } from './components/Header';
import { CommandPalette } from './components/CommandPalette';
import { ErrorState } from './components/ErrorState';
import { OverviewView } from './views/OverviewView';
import { TimelineView } from './views/TimelineView';
import { ProjectsView } from './views/ProjectsView';
import { AppsView } from './views/AppsView';
import { ResumptionView } from './views/ResumptionView';
import { SettingsView } from './views/SettingsView';

export const App: React.FC = () => {
  const [currentView, setCurrentView] = useState<NavView>('overview');
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>({
    state: 'connecting',
  });
  const [health, setHealth] = useState<HealthResult | undefined>(undefined);
  const [projects, setProjects] = useState<ProjectOverview[]>([]);
  const [timelineItems, setTimelineItems] = useState<TimelineItem[]>([]);
  const [resumptionPath, setResumptionPath] = useState<string>('');
  const [resumptionReport, setResumptionReport] = useState<ResumptionReport | null>(null);

  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [isSearchOpen, setIsSearchOpen] = useState<boolean>(false);
  const [isDark, setIsDark] = useState<boolean>(() => {
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  });

  // Filters state
  const [timelineDomain, setTimelineDomain] = useState<string>('all');
  const [timelineWindow, setTimelineWindow] = useState<string>('all');
  const [timelineLimit, setTimelineLimit] = useState<number>(50);
  const [projectsSort, setProjectsSort] = useState<'recent' | 'name' | 'activity'>('recent');

  // Dark mode effect
  useEffect(() => {
    if (isDark) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }, [isDark]);

  // Load backend data
  const fetchData = useCallback(async () => {
    if (!window.lifelog) return;
    setIsLoading(true);
    setError(null);

    try {
      // 1. Connection & Health
      const status = await window.lifelog.getConnectionStatus();
      setConnectionStatus(status);

      if (status.state === 'connected') {
        const h = await window.lifelog.getHealth();
        setHealth(h);

        // 2. Fetch Projects
        const projRes = await window.lifelog.getProjects({ sort: projectsSort, limit: 100 });
        setProjects(projRes.projects || []);

        // Default resumption path to first project if unset
        if (!resumptionPath && projRes.projects && projRes.projects.length > 0) {
          setResumptionPath(projRes.projects[0].canonical_path);
        }

        // 3. Fetch Timeline
        const tlRes = await window.lifelog.getTimeline({
          domain: timelineDomain as any,
          limit: timelineLimit,
          since: timelineWindow === 'all' ? undefined : timelineWindow,
        });
        setTimelineItems(tlRes.items || []);
      }
    } catch (err: any) {
      console.error('Failed to fetch data from Go daemon:', err);
      setError(err?.message || 'Unknown error communicating with Go daemon');
    } finally {
      setIsLoading(false);
    }
  }, [projectsSort, timelineDomain, timelineLimit, timelineWindow, resumptionPath]);

  // Initial load and connection status listener
  useEffect(() => {
    if (!window.lifelog) return;

    fetchData();

    const unsubscribe = window.lifelog.onConnectionStatusChange((status) => {
      setConnectionStatus(status);
      if (status.state === 'connected') {
        fetchData();
      }
    });

    return () => {
      unsubscribe();
    };
  }, [fetchData]);

  // Keyboard shortcut listener for Cmd+K
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setIsSearchOpen((prev) => !prev);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  // Project synthesis handler
  const handleSynthesize = async (path: string, gap: string) => {
    if (!window.lifelog || !path) return;
    setIsLoading(true);
    setError(null);
    try {
      const res = await window.lifelog.getResumption(path, { gap, limit: 30 });
      setResumptionReport(res.report);
      setResumptionPath(path);
      setCurrentView('resumption');
    } catch (err: any) {
      console.error('Failed to synthesize resumption report:', err);
      setError(err?.message || 'Synthesis failed');
    } finally {
      setIsLoading(false);
    }
  };

  const handleSelectProject = (path: string) => {
    setResumptionPath(path);
    setCurrentView('resumption');
    handleSynthesize(path, '30m');
  };

  // View titles and subtitles
  const viewMeta: Record<NavView, { title: string; subtitle: string }> = {
    overview: {
      title: "Today's Computer Memory",
      subtitle: 'Real-time overview of recorded activity across projects and applications',
    },
    timeline: {
      title: 'Activity Timeline',
      subtitle: 'Chronological feed of observed filesystem events, Git observations, and terminal executions',
    },
    projects: {
      title: 'Known Workspaces',
      subtitle: 'Local projects discovered from observed computer activity',
    },
    apps: {
      title: 'Application Usage',
      subtitle: 'Observed foreground application active time and session history',
    },
    resumption: {
      title: 'Context Resumption',
      subtitle: 'Synthesize contiguous work sessions to pick up exactly where you left off',
    },
    settings: {
      title: 'Settings & Diagnostics',
      subtitle: 'Daemon health, local storage boundaries, and V2 feature scope',
    },
  };

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-canvas-light dark:bg-canvas-dark text-primary-light dark:text-primary-dark">
      {/* Sidebar */}
      <Sidebar
        currentView={currentView}
        onNavigate={setCurrentView}
        onOpenSearch={() => setIsSearchOpen(true)}
        connectionStatus={connectionStatus}
        isDark={isDark}
        onToggleTheme={() => setIsDark((prev) => !prev)}
      />

      {/* Main Content Area */}
      <div className="flex-1 flex flex-col min-w-0 h-full overflow-hidden">
        <Header
          title={viewMeta[currentView].title}
          subtitle={viewMeta[currentView].subtitle}
          onRefresh={fetchData}
          isLoading={isLoading}
          onOpenSearch={() => setIsSearchOpen(true)}
        />

        {/* Scrollable View Container */}
        <main className="flex-1 overflow-y-auto">
          {error && (
            <div className="p-8 max-w-4xl mx-auto">
              <ErrorState
                title="Error querying local engine"
                message={error}
                onRetry={fetchData}
              />
            </div>
          )}

          {currentView === 'overview' && (
            <OverviewView
              projects={projects}
              timelineItems={timelineItems}
              health={health}
              isLoading={isLoading}
              onNavigate={setCurrentView}
              onSelectProject={handleSelectProject}
            />
          )}

          {currentView === 'timeline' && (
            <TimelineView
              items={timelineItems}
              isLoading={isLoading}
              onFilterChange={({ domain, window: win, limit }) => {
                if (domain !== undefined) setTimelineDomain(domain);
                if (win !== undefined) setTimelineWindow(win);
                if (limit !== undefined) setTimelineLimit(limit);
              }}
              activeDomain={timelineDomain}
              activeWindow={timelineWindow}
            />
          )}

          {currentView === 'projects' && (
            <ProjectsView
              projects={projects}
              isLoading={isLoading}
              onSelectProject={handleSelectProject}
              sortMode={projectsSort}
              onSortChange={setProjectsSort}
            />
          )}

          {currentView === 'apps' && (
            <AppsView />
          )}

          {currentView === 'resumption' && (
            <ResumptionView
              currentPath={resumptionPath}
              onPathChange={setResumptionPath}
              onSynthesize={handleSynthesize}
              report={resumptionReport}
              isLoading={isLoading}
              projects={projects}
            />
          )}

          {currentView === 'settings' && (
            <SettingsView
              health={health}
              connectionStatus={connectionStatus}
            />
          )}
        </main>
      </div>

      {/* Cmd+K Command Palette Modal */}
      <CommandPalette
        isOpen={isSearchOpen}
        onClose={() => setIsSearchOpen(false)}
        onNavigate={setCurrentView}
        projects={projects}
        onSelectProject={handleSelectProject}
      />
    </div>
  );
};
