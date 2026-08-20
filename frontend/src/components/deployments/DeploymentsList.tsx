'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'next/navigation';
import { deploymentsApi, environmentsApi } from '@/services/api';
import { cn, formatRelativeTime, formatDate } from '@/lib/utils';
import toast from 'react-hot-toast';
import { DeploymentDetailModal } from './DeploymentDetailModal';

interface Deployment {
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
}

interface Environment {
  id: string;
  name: string;
}

export function DeploymentsList() {
  const queryClient = useQueryClient();
  const params = useParams();
  const projectId = params.projectId as string;
  const [selectedDeployment, setSelectedDeployment] = useState<Deployment | null>(null);
  const [showDetail, setShowDetail] = useState(false);

  const { data: environments } = useQuery({
    queryKey: ['environments', projectId],
    queryFn: () => environmentsApi.list(projectId),
    select: (data) => data.data as Environment[],
    enabled: !!projectId,
  });

  const { data: deployments, isLoading, error, refetch } = useQuery({
    queryKey: ['deployments', projectId],
    queryFn: () => deploymentsApi.list(projectId),
    select: (data) => data.data as Deployment[],
    enabled: !!projectId,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (data?.some((d) => d.status === 'running' || d.status === 'pending')) {
        return 3000;
      }
      return false;
    },
  });

  const statusColors: Record<Deployment['status'], string> = {
    pending: 'bg-yellow-100 text-yellow-700',
    running: 'bg-blue-100 text-blue-700',
    success: 'bg-green-100 text-green-700',
    failed: 'bg-red-100 text-red-700',
    cancelled: 'bg-gray-100 text-gray-700',
  };

  const statusIcons: Record<Deployment['status'], React.ReactNode> = {
    pending: <svg className="h-4 w-4 animate-pulse" fill="currentColor" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10" /></svg>,
    running: <svg className="h-4 w-4 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24"><circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" /><path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" /></svg>,
    success: <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 24 24"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" /></svg>,
    failed: <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z" /></svg>,
    cancelled: <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm5 13.59L15.59 17 12 13.41 8.41 17 7 15.59 10.59 12 7 8.41 8.41 7 12 10.59 15.59 7 17 8.41 13.41 12 17 15.59z" /></svg>,
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        {[...Array(5)].map((_, i) => (
          <div key={i} className="animate-pulse">
            <div className="h-16 bg-gray-200 rounded-lg" />
          </div>
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
        </svg>
        <h3 className="mt-2 text-sm font-medium text-gray-900">Failed to load deployments</h3>
        <p className="mt-1 text-sm text-gray-500">{error instanceof Error ? error.message : 'Unknown error'}</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Deployments</h1>
          <p className="mt-1 text-sm text-gray-500">Deployment history and status</p>
        </div>
        <button
          onClick={() => refetch()}
          disabled={deployments?.some((d) => d.status === 'running' || d.status === 'pending')}
          className="inline-flex items-center gap-2 px-4 py-2 border border-gray-300 text-sm font-medium rounded-lg text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 transition-colors"
        >
          <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          Refresh
        </button>
      </div>

      {/* Stats Summary */}
      {deployments && deployments.length > 0 && (
        <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
          <StatCard label="Total" value={deployments.length} />
          <StatCard
            label="Running"
            value={deployments.filter((d) => d.status === 'running').length}
            color="blue"
          />
          <StatCard
            label="Success"
            value={deployments.filter((d) => d.status === 'success').length}
            color="green"
          />
          <StatCard
            label="Failed"
            value={deployments.filter((d) => d.status === 'failed').length}
            color="red"
          />
          <StatCard
            label="Pending"
            value={deployments.filter((d) => d.status === 'pending').length}
            color="yellow"
          />
        </div>
      )}

      {/* Deployments Table */}
      {deployments && deployments.length === 0 ? (
        <div className="text-center py-12">
          <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 17h8m0 0V9m0 8l-8-8-4 4-6-6" />
          </svg>
          <h3 className="mt-2 text-sm font-medium text-gray-900">No deployments yet</h3>
          <p className="mt-1 text-sm text-gray-500">Deployments will appear here when you trigger them from environments.</p>
        </div>
      ) : (
        <div className="bg-white rounded-xl border border-gray-200 shadow-sm overflow-hidden">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Environment</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Commit</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Branch</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Started</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Duration</th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {deployments?.map((deployment) => (
                <tr key={deployment.id} className="hover:bg-gray-50 cursor-pointer" onClick={() => { setSelectedDeployment(deployment); setShowDetail(true); }}>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <span className={cn('inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium', statusColors[deployment.status])}>
                      {statusIcons[deployment.status]}
                      {deployment.status.charAt(0).toUpperCase() + deployment.status.slice(1)}
                    </span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <div className="text-sm font-medium text-gray-900">{deployment.environment_name}</div>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <code className="text-sm font-mono text-gray-900">{deployment.commit_sha.slice(0, 8)}</code>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    {deployment.branch ? (
                      <span className="text-sm text-gray-500 font-mono">{deployment.branch}</span>
                    ) : (
                      <span className="text-sm text-gray-400">—</span>
                    )}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                    {formatRelativeTime(deployment.started_at)}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                    {deployment.completed_at ? (
                      <>
                        {Math.round((new Date(deployment.completed_at).getTime() - new Date(deployment.started_at).getTime()) / 1000)}s
                      </>
                    ) : deployment.status === 'running' ? (
                      <span className="text-blue-600">In progress...</span>
                    ) : (
                      '—'
                    )}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                    <button
                      onClick={(e) => { e.stopPropagation(); setSelectedDeployment(deployment); setShowDetail(true); }}
                      className="text-primary-600 hover:text-primary-900"
                    >
                      View
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Deployment Detail Modal */}
      {showDetail && selectedDeployment && (
        <DeploymentDetailModal
          deployment={selectedDeployment}
          onClose={() => setShowDetail(false)}
          onRefetch={refetch}
        />
      )}
    </div>
  );
}

function StatCard({ label, value, color = 'gray' }: { label: string; value: number; color?: string }) {
  const colors = {
    gray: 'bg-gray-100 text-gray-700',
    blue: 'bg-blue-100 text-blue-700',
    green: 'bg-green-100 text-green-700',
    red: 'bg-red-100 text-red-700',
    yellow: 'bg-yellow-100 text-yellow-700',
  };
  return (
    <div className="p-4 bg-white rounded-lg border border-gray-200">
      <p className="text-xs font-medium text-gray-500 uppercase tracking-wider">{label}</p>
      <p className="mt-1 text-3xl font-bold text-gray-900">{value}</p>
    </div>
  );
}