'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'next/navigation';
import { aiApi, projectsApi } from '@/services/api';
import { cn, formatRelativeTime } from '@/lib/utils';
import toast from 'react-hot-toast';
import {
  Send,
  FileText,
  TestTube,
  ArrowRightLeft,
  Loader2,
  CheckCircle,
  XCircle,
  MessageSquare,
  Sparkles,
} from 'lucide-react';

type AITaskType = 'review' | 'docs' | 'tests' | 'migrate';
type AITaskStatus = 'pending' | 'running' | 'completed' | 'failed';

interface AIJob {
  id: string;
  project_id: string;
  type: AITaskType;
  status: AITaskStatus;
  input: Record<string, unknown>;
  result: Record<string, unknown> | null;
  error: string | null;
  created_at: string;
  updated_at: string;
  completed_at: string | null;
}

interface Project {
  id: string;
  name: string;
}

const taskConfig: Record<AITaskType, { label: string; icon: React.ReactNode; color: string }> = {
  review: { label: 'PR Review', icon: <MessageSquare className="h-5 w-5" />, color: 'blue' },
  docs: { label: 'Generate Docs', icon: <FileText className="h-5 w-5" />, color: 'green' },
  tests: { label: 'Generate Tests', icon: <TestTube className="h-5 w-5" />, color: 'purple' },
  migrate: { label: 'Migrate Code', icon: <ArrowRightLeft className="h-5 w-5" />, color: 'orange' },
};

const statusColors: Record<AITaskStatus, string> = {
  pending: 'bg-gray-100 text-gray-700',
  running: 'bg-blue-100 text-blue-700',
  completed: 'bg-green-100 text-green-700',
  failed: 'bg-red-100 text-red-700',
};

export function AIAssistant() {
  const queryClient = useQueryClient();
  const params = useParams();
  const projectId = params.projectId as string;
  const [activeTask, setActiveTask] = useState<AITaskType>('review');
  const [showResult, setShowResult] = useState<AIJob | null>(null);

  const { data: project } = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => projectsApi.get(projectId),
    select: (data) => data.data as Project,
    enabled: !!projectId,
  });

  const { data: jobs, isLoading } = useQuery({
    queryKey: ['ai-jobs', projectId],
    queryFn: () => aiApi.listJobs(projectId),
    select: (data) => data.data as AIJob[],
    enabled: !!projectId,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (data?.some((j) => j.status === 'running' || j.status === 'pending')) {
        return 3000;
      }
      return false;
    },
  });

  const reviewMutation = useMutation({
    mutationFn: (data: { pr_number: number; repo_url: string; base_branch: string; head_branch: string; files?: string[]; focus_areas?: string[] }) =>
      aiApi.reviewPR(projectId, data),
    onSuccess: (job) => {
      queryClient.invalidateQueries({ queryKey: ['ai-jobs', projectId] });
      setShowResult(job.data);
      toast.success('PR review started');
    },
    onError: (error: unknown) => {
      toast.error(error instanceof Error ? error.message : 'Failed to start PR review');
    },
  });

  const docsMutation = useMutation({
    mutationFn: (data: { repo_url: string; paths?: string[]; doc_types?: string[]; format?: string }) =>
      aiApi.generateDocs(projectId, data),
    onSuccess: (job) => {
      queryClient.invalidateQueries({ queryKey: ['ai-jobs', projectId] });
      setShowResult(job.data);
      toast.success('Documentation generation started');
    },
    onError: (error: unknown) => {
      toast.error(error instanceof Error ? error.message : 'Failed to generate docs');
    },
  });

  const testsMutation = useMutation({
    mutationFn: (data: { repo_url: string; paths?: string[]; test_types?: string[]; framework?: string; coverage?: number }) =>
      aiApi.generateTests(projectId, data),
    onSuccess: (job) => {
      queryClient.invalidateQueries({ queryKey: ['ai-jobs', projectId] });
      setShowResult(job.data);
      toast.success('Test generation started');
    },
    onError: (error: unknown) => {
      toast.error(error instanceof Error ? error.message : 'Failed to generate tests');
    },
  });

  const migrateMutation = useMutation({
    mutationFn: (data: { repo_url: string; source_lang: string; target_lang: string; paths?: string[]; preserve_tests?: boolean }) =>
      aiApi.migrateCode(projectId, data),
    onSuccess: (job) => {
      queryClient.invalidateQueries({ queryKey: ['ai-jobs', projectId] });
      setShowResult(job.data);
      toast.success('Code migration started');
    },
    onError: (error: unknown) => {
      toast.error(error instanceof Error ? error.message : 'Failed to start migration');
    },
  });

  const isAnyLoading = reviewMutation.isPending || docsMutation.isPending || testsMutation.isPending || migrateMutation.isPending;

  // Forms state
  const [reviewForm, setReviewForm] = useState({
    pr_number: '',
    repo_url: '',
    base_branch: 'main',
    head_branch: '',
    files: '',
    focus_areas: '',
  });
  const [docsForm, setDocsForm] = useState({
    repo_url: '',
    paths: '',
    doc_types: 'readme,api,architecture',
    format: 'markdown',
  });
  const [testsForm, setTestsForm] = useState({
    repo_url: '',
    paths: '',
    test_types: 'unit,integration',
    framework: 'jest',
    coverage: 80,
  });
  const [migrateForm, setMigrateForm] = useState({
    repo_url: '',
    source_lang: 'javascript',
    target_lang: 'typescript',
    paths: '',
    preserve_tests: true,
  });

  const handleReviewSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    reviewMutation.mutate({
      pr_number: parseInt(reviewForm.pr_number),
      repo_url: reviewForm.repo_url,
      base_branch: reviewForm.base_branch,
      head_branch: reviewForm.head_branch,
      files: reviewForm.files ? reviewForm.files.split(',').map((s) => s.trim()) : undefined,
      focus_areas: reviewForm.focus_areas ? reviewForm.focus_areas.split(',').map((s) => s.trim()) : undefined,
    });
  };

  const handleDocsSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    docsMutation.mutate({
      repo_url: docsForm.repo_url,
      paths: docsForm.paths ? docsForm.paths.split(',').map((s) => s.trim()) : undefined,
      doc_types: docsForm.doc_types.split(',').map((s) => s.trim()),
      format: docsForm.format,
    });
  };

  const handleTestsSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    testsMutation.mutate({
      repo_url: testsForm.repo_url,
      paths: testsForm.paths ? testsForm.paths.split(',').map((s) => s.trim()) : undefined,
      test_types: testsForm.test_types.split(',').map((s) => s.trim()),
      framework: testsForm.framework,
      coverage: testsForm.coverage,
    });
  };

  const handleMigrateSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    migrateMutation.mutate({
      repo_url: migrateForm.repo_url,
      source_lang: migrateForm.source_lang,
      target_lang: migrateForm.target_lang,
      paths: migrateForm.paths ? migrateForm.paths.split(',').map((s) => s.trim()) : undefined,
      preserve_tests: migrateForm.preserve_tests,
    });
  };

  if (!projectId) {
    return (
      <div className="text-center py-12">
        <Sparkles className="mx-auto h-12 w-12 text-gray-400" />
        <h3 className="mt-2 text-sm font-medium text-gray-900">Select a project</h3>
        <p className="mt-1 text-sm text-gray-500">Choose a project to use AI-assisted workflows.</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 flex items-center gap-2">
            <Sparkles className="h-6 w-6 text-primary-600" />
            AI Assistant
          </h1>
          <p className="mt-1 text-sm text-gray-500">
            {project ? `AI-powered workflows for ${project.name}` : 'Automate reviews, docs, tests, and migrations'}
          </p>
        </div>
      </div>

      {/* Task Type Tabs */}
      <div className="bg-white rounded-xl border border-gray-200 shadow-sm overflow-hidden">
        <div className="border-b border-gray-200 p-4">
          <nav className="flex gap-2 overflow-x-auto" aria-label="AI task types">
            {(['review', 'docs', 'tests', 'migrate'] as AITaskType[]).map((task) => (
              <button
                key={task}
                onClick={() => { setActiveTask(task); setShowResult(null); }}
                className={cn(
                  'flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-lg whitespace-nowrap transition-colors',
                  activeTask === task
                    ? 'bg-primary-50 text-primary-700 border border-primary-200'
                    : 'text-gray-600 hover:text-gray-900 hover:bg-gray-50'
                )}
              >
                {taskConfig[task].icon}
                {taskConfig[task].label}
              </button>
            ))}
          </nav>
        </div>

        {/* Task Form */}
        <div className="p-4">
          {activeTask === 'review' && (
            <form onSubmit={handleReviewSubmit} className="space-y-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">PR Number *</label>
                  <input
                    type="number"
                    value={reviewForm.pr_number}
                    onChange={(e) => setReviewForm({ ...reviewForm, pr_number: e.target.value })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                    placeholder="123"
                    required
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Repository URL *</label>
                  <input
                    type="url"
                    value={reviewForm.repo_url}
                    onChange={(e) => setReviewForm({ ...reviewForm, repo_url: e.target.value })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                    placeholder="https://github.com/owner/repo"
                    required
                  />
                </div>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Base Branch</label>
                  <input
                    type="text"
                    value={reviewForm.base_branch}
                    onChange={(e) => setReviewForm({ ...reviewForm, base_branch: e.target.value })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                    placeholder="main"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Head Branch *</label>
                  <input
                    type="text"
                    value={reviewForm.head_branch}
                    onChange={(e) => setReviewForm({ ...reviewForm, head_branch: e.target.value })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                    placeholder="feature-branch"
                    required
                  />
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Files (comma-separated, optional)</label>
                <input
                  type="text"
                  value={reviewForm.files}
                  onChange={(e) => setReviewForm({ ...reviewForm, files: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="src/auth.ts, src/api.ts"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Focus Areas (comma-separated, optional)</label>
                <input
                  type="text"
                  value={reviewForm.focus_areas}
                  onChange={(e) => setReviewForm({ ...reviewForm, focus_areas: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="security,performance,style"
                />
              </div>
              <button
                type="submit"
                disabled={isAnyLoading || !reviewForm.pr_number || !reviewForm.repo_url || !reviewForm.head_branch}
                className="w-full sm:w-auto px-6 py-2.5 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-2"
              >
                {reviewMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                {reviewMutation.isPending ? 'Reviewing...' : 'Start Review'}
              </button>
            </form>
          )}

          {activeTask === 'docs' && (
            <form onSubmit={handleDocsSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Repository URL *</label>
                <input
                  type="url"
                  value={docsForm.repo_url}
                  onChange={(e) => setDocsForm({ ...docsForm, repo_url: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="https://github.com/owner/repo"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Paths (comma-separated, optional)</label>
                <input
                  type="text"
                  value={docsForm.paths}
                  onChange={(e) => setDocsForm({ ...docsForm, paths: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="src/, docs/"
                />
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Doc Types</label>
                  <input
                    type="text"
                    value={docsForm.doc_types}
                    onChange={(e) => setDocsForm({ ...docsForm, doc_types: e.target.value })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                    placeholder="readme,api,architecture"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Format</label>
                  <select
                    value={docsForm.format}
                    onChange={(e) => setDocsForm({ ...docsForm, format: e.target.value })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  >
                    <option value="markdown">Markdown</option>
                    <option value="html">HTML</option>
                    <option value="pdf">PDF</option>
                  </select>
                </div>
              </div>
              <button
                type="submit"
                disabled={isAnyLoading || !docsForm.repo_url}
                className="w-full sm:w-auto px-6 py-2.5 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-2"
              >
                {docsMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                {docsMutation.isPending ? 'Generating...' : 'Generate Docs'}
              </button>
            </form>
          )}

          {activeTask === 'tests' && (
            <form onSubmit={handleTestsSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Repository URL *</label>
                <input
                  type="url"
                  value={testsForm.repo_url}
                  onChange={(e) => setTestsForm({ ...testsForm, repo_url: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="https://github.com/owner/repo"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Paths (comma-separated, optional)</label>
                <input
                  type="text"
                  value={testsForm.paths}
                  onChange={(e) => setTestsForm({ ...testsForm, paths: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="src/utils/, src/api/"
                />
              </div>
              <div className="grid gap-4 sm:grid-cols-3">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Test Types</label>
                  <input
                    type="text"
                    value={testsForm.test_types}
                    onChange={(e) => setTestsForm({ ...testsForm, test_types: e.target.value })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                    placeholder="unit,integration,e2e"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Framework</label>
                  <select
                    value={testsForm.framework}
                    onChange={(e) => setTestsForm({ ...testsForm, framework: e.target.value })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  >
                    <option value="jest">Jest</option>
                    <option value="vitest">Vitest</option>
                    <option value="pytest">pytest</option>
                    <option value="go test">Go test</option>
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Target Coverage %</label>
                  <input
                    type="number"
                    min="0"
                    max="100"
                    value={testsForm.coverage}
                    onChange={(e) => setTestsForm({ ...testsForm, coverage: parseInt(e.target.value) })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  />
                </div>
              </div>
              <button
                type="submit"
                disabled={isAnyLoading || !testsForm.repo_url}
                className="w-full sm:w-auto px-6 py-2.5 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-2"
              >
                {testsMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                {testsMutation.isPending ? 'Generating...' : 'Generate Tests'}
              </button>
            </form>
          )}

          {activeTask === 'migrate' && (
            <form onSubmit={handleMigrateSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Repository URL *</label>
                <input
                  type="url"
                  value={migrateForm.repo_url}
                  onChange={(e) => setMigrateForm({ ...migrateForm, repo_url: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="https://github.com/owner/repo"
                  required
                />
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Source Language *</label>
                  <select
                    value={migrateForm.source_lang}
                    onChange={(e) => setMigrateForm({ ...migrateForm, source_lang: e.target.value })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  >
                    <option value="javascript">JavaScript</option>
                    <option value="typescript">TypeScript</option>
                    <option value="python">Python</option>
                    <option value="go">Go</option>
                    <option value="rust">Rust</option>
                    <option value="java">Java</option>
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Target Language *</label>
                  <select
                    value={migrateForm.target_lang}
                    onChange={(e) => setMigrateForm({ ...migrateForm, target_lang: e.target.value })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  >
                    <option value="typescript">TypeScript</option>
                    <option value="javascript">JavaScript</option>
                    <option value="python">Python</option>
                    <option value="go">Go</option>
                    <option value="rust">Rust</option>
                    <option value="java">Java</option>
                  </select>
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Paths (comma-separated, optional)</label>
                <input
                  type="text"
                  value={migrateForm.paths}
                  onChange={(e) => setMigrateForm({ ...migrateForm, paths: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="src/"
                />
              </div>
              <div className="flex items-center">
                <input
                  type="checkbox"
                  id="preserve_tests"
                  checked={migrateForm.preserve_tests}
                  onChange={(e) => setMigrateForm({ ...migrateForm, preserve_tests: e.target.checked })}
                  className="h-4 w-4 text-primary-600 border-gray-300 rounded focus:ring-primary-500"
                />
                <label htmlFor="preserve_tests" className="ml-2 text-sm text-gray-700">Preserve existing tests</label>
              </div>
              <button
                type="submit"
                disabled={isAnyLoading || !migrateForm.repo_url}
                className="w-full sm:w-auto px-6 py-2.5 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-2"
              >
                {migrateMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                {migrateMutation.isPending ? 'Migrating...' : 'Start Migration'}
              </button>
            </form>
          )}
        </div>

        {/* Recent Jobs */}
        <div className="border-t border-gray-200 p-4">
          <h3 className="text-sm font-medium text-gray-900 mb-3">Recent Jobs</h3>
          {isLoading ? (
            <div className="space-y-3">
              {[...Array(3)].map((_, i) => (
                <div key={i} className="animate-pulse h-16 bg-gray-200 rounded-lg" />
              ))}
            </div>
          ) : jobs && jobs.length === 0 ? (
            <p className="text-center text-gray-500 py-8">No AI jobs yet. Start one above!</p>
          ) : (
            <div className="space-y-2 max-h-64 overflow-auto">
              {jobs?.slice(0, 10).map((job) => (
                <JobCard key={job.id} job={job} onClick={() => setShowResult(job)} />
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Result Modal */}
      {showResult && (
        <AIResultModal job={showResult} onClose={() => setShowResult(null)} />
      )}
    </div>
  );
}

interface JobCardProps {
  job: AIJob;
  onClick: () => void;
}

function JobCard({ job, onClick }: JobCardProps) {
  const config = taskConfig[job.type];
  return (
    <button
      onClick={onClick}
      className="w-full text-left p-3 bg-gray-50 rounded-lg hover:bg-gray-100 transition-colors flex items-center gap-3"
    >
      <div className={cn('p-2 rounded-lg', `bg-${config.color}-100 text-${config.color}-700`)}>
        {config.icon}
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-gray-900 truncate">{config.label}</p>
        <p className="text-xs text-gray-500 truncate">
          {job.input.repo_url ? new URL(job.input.repo_url as string).pathname.slice(1) : 'Unknown repo'}
        </p>
      </div>
      <div className="flex items-center gap-2">
        <span className={cn('px-2 py-0.5 rounded-full text-xs font-medium', statusColors[job.status])}>
          {job.status.charAt(0).toUpperCase() + job.status.slice(1)}
        </span>
        <span className="text-xs text-gray-400">{formatRelativeTime(job.created_at)}</span>
      </div>
    </button>
  );
}

interface AIResultModalProps {
  job: AIJob;
  onClose: () => void;
}

function AIResultModal({ job, onClose }: AIResultModalProps) {
  const config = taskConfig[job.type];

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto" role="dialog" aria-modal="true">
      <div className="flex min-h-full items-center justify-center p-4">
        <div className="fixed inset-0 bg-gray-900/50" onClick={onClose} aria-hidden="true" />
        <div className="relative w-full max-w-3xl bg-white rounded-xl shadow-xl max-h-[80vh] flex flex-col">
          <div className="flex items-center justify-between p-4 border-b border-gray-200 sticky top-0 bg-white">
            <div className="flex items-center gap-3">
              <div className={cn('p-2 rounded-lg', `bg-${config.color}-100 text-${config.color}-700`)}>
                {config.icon}
              </div>
              <div>
                <h3 className="text-lg font-semibold text-gray-900">{config.label} Result</h3>
                <p className="text-sm text-gray-500">Job ID: {job.id.slice(0, 12)}</p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <span className={cn('px-3 py-1 rounded-full text-xs font-medium', statusColors[job.status])}>
                {job.status.charAt(0).toUpperCase() + job.status.slice(1)}
              </span>
              <button onClick={onClose} className="p-1 text-gray-400 hover:text-gray-600 rounded">
                <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>

          <div className="flex-1 overflow-auto p-4">
            {job.error && (
              <div className="mb-4 p-4 bg-red-50 border border-red-200 rounded-lg text-red-700">
                <p className="font-medium">Error:</p>
                <pre className="mt-1 text-sm whitespace-pre-wrap">{job.error}</pre>
              </div>
            )}

            {job.result && (
              <div className="space-y-4">
                {Object.entries(job.result).map(([key, value]) => (
                  <div key={key} className="border border-gray-200 rounded-lg overflow-hidden">
                    <div className="px-4 py-2 bg-gray-50 border-b border-gray-200 font-medium text-gray-900 capitalize">
                      {key.replace(/_/g, ' ')}
                    </div>
                    <div className="p-4 max-h-96 overflow-auto">
                      <pre className="text-sm whitespace-pre-wrap font-mono text-gray-700">
                        {typeof value === 'string' ? value : JSON.stringify(value, null, 2)}
                      </pre>
                    </div>
                  </div>
                ))}
              </div>
            )}

            {!job.result && !job.error && job.status !== 'completed' && (
              <div className="text-center py-12 text-gray-500">
                <Loader2 className="mx-auto h-8 w-8 animate-spin text-primary-600 mb-2" />
                <p>Job is still running...</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}