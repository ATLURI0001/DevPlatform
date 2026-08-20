'use client';

import { useEffect, useState, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import { X, Download, Copy, Check } from 'lucide-react';
import { deploymentsApi } from '@/services/api';
import { cn, formatDate } from '@/lib/utils';
import toast from 'react-hot-toast';

interface DeploymentDetailModalProps {
  deployment: {
    id: string;
    project_id: string;
    environment_id: string;
    environment_name: string;
    commit_sha: string;
    branch: string | null;
    status: 'pending' | 'running' | 'success' | 'failed' | 'cancelled';
    started_at: string;
    completed_at: string | null;
    logs: string | null;
  };
  onClose: () => void;
  onRefetch: () => void;
}

export function DeploymentDetailModal({ deployment, onClose, onRefetch }: DeploymentDetailModalProps) {
  const [copied, setCopied] = useState(false);
  const [logLines, setLogLines] = useState<string[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const logsEndRef = useRef<HTMLDivElement>(null);

  const statusColors: Record<typeof deployment.status, string> = {
    pending: 'bg-yellow-100 text-yellow-700',
    running: 'bg-blue-100 text-blue-700',
    success: 'bg-green-100 text-green-700',
    failed: 'bg-red-100 text-red-700',
    cancelled: 'bg-gray-100 text-gray-700',
  };

  // Fetch logs if not already loaded
  const { data: logsData, isLoading: logsLoading } = useQuery({
    queryKey: ['deployment-logs', deployment.project_id, deployment.id],
    queryFn: () => deploymentsApi.getLogs(deployment.project_id, deployment.id),
    select: (data) => data.data as { logs: string },
    enabled: !deployment.logs && (deployment.status === 'running' || deployment.status === 'failed'),
    refetchInterval: deployment.status === 'running' ? 2000 : false,
  });

  // Combine logs from deployment and fetched data
  const allLogs = deployment.logs || logsData?.logs || '';

  useEffect(() => {
    if (allLogs) {
      setLogLines(allLogs.split('\n').filter((line) => line.trim()));
    }
  }, [allLogs]);

  useEffect(() => {
    if (autoScroll && logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logLines, autoScroll]);

  const copyLogs = async () => {
    try {
      await navigator.clipboard.writeText(allLogs);
      setCopied(true);
      toast.success('Logs copied to clipboard');
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error('Failed to copy logs');
    }
  };

  const downloadLogs = () => {
    const blob = new Blob([allLogs], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `deployment-${deployment.id.slice(0, 8)}-logs.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    toast.success('Logs downloaded');
  };

  const formatLogLine = (line: string) => {
    // Simple log formatting - highlight timestamps, levels, etc.
    return line
      .replace(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d{3}Z)\s*/, '<span class="text-gray-400">$1</span> ')
      .replace(/\b(ERROR|FATAL|CRITICAL)\b/g, '<span class="text-red-500 font-medium">$1</span>')
      .replace(/\b(WARN|WARNING)\b/g, '<span class="text-yellow-500 font-medium">$1</span>')
      .replace(/\b(INFO)\b/g, '<span class="text-blue-500 font-medium">$1</span>')
      .replace(/\b(DEBUG|TRACE)\b/g, '<span class="text-gray-500">$1</span>')
      .replace(/\b(SUCCESS|OK|COMPLETE)\b/g, '<span class="text-green-500 font-medium">$1</span>');
  };

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto" role="dialog" aria-modal="true" aria-labelledby="modal-title">
      <div className="flex min-h-full items-center justify-center p-4">
        <div className="fixed inset-0 bg-gray-900/50 transition-opacity" onClick={onClose} aria-hidden="true" />

        <div className="relative w-full max-w-4xl bg-white rounded-xl shadow-xl transform transition-all max-h-[90vh] flex flex-col">
          {/* Header */}
          <div className="flex items-center justify-between p-4 border-b border-gray-200 sticky top-0 bg-white z-10">
            <div>
              <h2 id="modal-title" className="text-lg font-semibold text-gray-900">Deployment Details</h2>
              <p className="text-sm text-gray-500">{deployment.environment_name} • {deployment.id.slice(0, 12)}</p>
            </div>
            <div className="flex items-center gap-2">
              <span className={cn('px-3 py-1 rounded-full text-xs font-medium', statusColors[deployment.status])}>
                {deployment.status.charAt(0).toUpperCase() + deployment.status.slice(1)}
              </span>
              <button
                onClick={onClose}
                className="p-1 text-gray-400 hover:text-gray-600 rounded-md hover:bg-gray-100 transition-colors"
                aria-label="Close modal"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
          </div>

          {/* Content */}
          <div className="flex-1 overflow-hidden flex flex-col">
            {/* Tabs */}
            <div className="border-b border-gray-200 px-4">
              <nav className="flex gap-1" aria-label="Deployment detail tabs">
                <TabButton
                  active={true}
                  onClick={() => setActiveTab('overview')}
                  label="Overview"
                />
                <TabButton
                  active={false}
                  onClick={() => setActiveTab('logs')}
                  label="Logs"
                  badge={logLines.length}
                />
              </nav>
            </div>

            {/* Tab Panels */}
            <div className="flex-1 overflow-auto p-4">
              {activeTab === 'overview' && (
                <div className="space-y-6">
                  {/* Deployment Info */}
                  <div className="grid gap-4 sm:grid-cols-2">
                    <InfoRow label="Deployment ID" value={deployment.id} copyable />
                    <InfoRow label="Environment" value={deployment.environment_name} />
                    <InfoRow label="Commit SHA" value={deployment.commit_sha} copyable monospace />
                    <InfoRow label="Branch" value={deployment.branch || '—'} monospace />
                    <InfoRow label="Status" value={<StatusBadge status={deployment.status} />} />
                    <InfoRow label="Started At" value={formatDate(deployment.started_at)} />
                    <InfoRow label="Completed At" value={deployment.completed_at ? formatDate(deployment.completed_at) : '—'} />
                    <InfoRow
                      label="Duration"
                      value={
                        deployment.completed_at
                          ? `${Math.round((new Date(deployment.completed_at).getTime() - new Date(deployment.started_at).getTime()) / 1000)}s`
                          : deployment.status === 'running'
                          ? 'In progress...'
                          : '—'
                      }
                    />
                  </div>

                  {/* Actions */}
                  <div className="pt-4 border-t border-gray-200 flex gap-3">
                    <button
                      onClick={() => { onRefetch(); onClose(); }}
                      className="px-4 py-2 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 transition-colors"
                    >
                      Refresh Status
                    </button>
                    {deployment.status === 'running' && (
                      <button className="px-4 py-2 border border-gray-300 text-sm font-medium rounded-lg text-gray-700 bg-white hover:bg-gray-50 transition-colors">
                        Cancel Deployment
                      </button>
                    )}
                  </div>
                </div>
              )}

              {activeTab === 'logs' && (
                <div className="space-y-4">
                  {/* Log Controls */}
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <label className="flex items-center gap-2 text-sm text-gray-700">
                        <input
                          type="checkbox"
                          checked={autoScroll}
                          onChange={(e) => setAutoScroll(e.target.checked)}
                          className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                        />
                        Auto-scroll
                      </label>
                      {logsLoading && (
                        <span className="flex items-center gap-1 text-sm text-blue-600">
                          <svg className="animate-spin h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                          </svg>
                          Streaming...
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-2">
                      <button
                        onClick={copyLogs}
                        className="flex items-center gap-1.5 px-3 py-1.5 border border-gray-300 text-sm font-medium rounded-lg text-gray-700 bg-white hover:bg-gray-50 transition-colors"
                      >
                        <Copy className="h-4 w-4" />
                        {copied ? <Check className="h-4 w-4 text-green-500" /> : 'Copy'}
                      </button>
                      <button
                        onClick={downloadLogs}
                        className="flex items-center gap-1.5 px-3 py-1.5 border border-gray-300 text-sm font-medium rounded-lg text-gray-700 bg-white hover:bg-gray-50 transition-colors"
                      >
                        <Download className="h-4 w-4" />
                        Download
                      </button>
                    </div>
                  </div>

                  {/* Log Output */}
                  <div className="bg-gray-950 rounded-lg p-4 font-mono text-sm text-gray-100 max-h-[50vh] overflow-auto" style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }}>
                    {logLines.length === 0 ? (
                      <div className="text-gray-500 text-center py-8">
                        {logsLoading ? 'Loading logs...' : 'No logs available yet'}
                      </div>
                    ) : (
                      <>
                        {logLines.map((line, index) => (
                          <div
                            key={index}
                            className="whitespace-pre-wrap break-all"
                            dangerouslySetInnerHTML={{ __html: formatLogLine(line) }}
                          />
                        ))}
                        <div ref={logsEndRef} />
                      </>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

const TabButton = ({ active, onClick, label, badge }: { active: boolean; onClick: () => void; label: string; badge?: number }) => (
  <button
    onClick={onClick}
    className={`px-4 py-2 text-sm font-medium rounded-t-lg transition-colors ${
      active
        ? 'text-primary-600 border-b-2 border-primary-600 bg-primary-50'
        : 'text-gray-500 hover:text-gray-700 hover:bg-gray-50'
    }`}
  >
    <span className="flex items-center gap-1.5">
      {label}
      {badge !== undefined && badge > 0 && (
        <span className={cn('px-1.5 py-0.5 text-xs font-medium rounded-full', active ? 'bg-primary-100 text-primary-700' : 'bg-gray-100 text-gray-600')}>
          {badge}
        </span>
      )}
    </span>
  </button>
);

const InfoRow = ({ label, value, copyable, monospace }: { label: string; value: React.ReactNode; copyable?: boolean; monospace?: boolean }) => (
  <div>
    <p className="text-xs font-medium text-gray-500 uppercase tracking-wider">{label}</p>
    <div className="mt-1 flex items-center gap-2">
      <span className={cn('text-sm text-gray-900 break-all', monospace && 'font-mono')}>{value}</span>
      {copyable && typeof value === 'string' && (
        <button
          onClick={() => {
            navigator.clipboard.writeText(value);
            toast.success('Copied');
          }}
          className="p-1 text-gray-400 hover:text-gray-600 rounded hover:bg-gray-100"
          aria-label="Copy to clipboard"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 012-2h10a2 2 0 012 2v1M8 5v15" />
          </svg>
        </button>
      )}
    </div>
  </div>
);

const StatusBadge = ({ status }: { status: typeof deployment.status }) => {
  const colors: Record<typeof deployment.status, string> = {
    pending: 'bg-yellow-100 text-yellow-700',
    running: 'bg-blue-100 text-blue-700',
    success: 'bg-green-100 text-green-700',
    failed: 'bg-red-100 text-red-700',
    cancelled: 'bg-gray-100 text-gray-700',
  };
  return (
    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${colors[status]}`}>
      {status.charAt(0).toUpperCase() + status.slice(1)}
    </span>
  );
};

