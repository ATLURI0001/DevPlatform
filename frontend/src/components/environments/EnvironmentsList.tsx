'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'next/navigation';
import { environmentsApi, projectsApi } from '@/services/api';
import { cn, formatRelativeTime, getInitials } from '@/lib/utils';
import toast from 'react-hot-toast';
import { CreateEnvironmentModal } from './CreateEnvironmentModal';

interface Environment {
  id: string;
  project_id: string;
  name: string;
  namespace: string;
  cluster: string | null;
  config: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
  last_deployment?: {
    id: string;
    status: string;
    created_at: string;
  } | null;
}

interface Project {
  id: string;
  name: string;
}

export function EnvironmentsList() {
  const queryClient = useQueryClient();
  const params = useParams();
  const projectId = params.projectId as string;
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [selectedEnvironment, setSelectedEnvironment] = useState<Environment | null>(null);

  const { data: project } = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => projectsApi.get(projectId),
    select: (data) => data.data as Project,
    enabled: !!projectId,
  });

  const { data: environments, isLoading, error } = useQuery({
    queryKey: ['environments', projectId],
    queryFn: () => environmentsApi.list(projectId),
    select: (data) => data.data as Environment[],
    enabled: !!projectId,
  });

  const createMutation = useMutation({
    mutationFn: (data: { name: string; namespace: string; cluster?: string; config?: Record<string, unknown> }) =>
      environmentsApi.create(projectId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments', projectId] });
      toast.success('Environment created successfully');
      setShowCreateModal(false);
    },
    onError: (error: unknown) => {
      const message = error instanceof Error ? error.message : 'Failed to create environment';
      toast.error(message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (envId: string) => environmentsApi.delete(projectId, envId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments', projectId] });
      toast.success('Environment deleted');
    },
    onError: (error: unknown) => {
      const message = error instanceof Error ? error.message : 'Failed to delete environment';
      toast.error(message);
    },
  });

  const deployMutation = useMutation({
    mutationFn: (data: { envId: string; commit_sha: string; branch?: string }) =>
      environmentsApi.deploy(projectId, data.envId, { commit_sha: data.commit_sha, branch: data.branch }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments', projectId] });
      queryClient.invalidateQueries({ queryKey: ['deployments', projectId] });
      toast.success('Deployment triggered');
    },
    onError: (error: unknown) => {
      const message = error instanceof Error ? error.message : 'Failed to trigger deployment';
      toast.error(message);
    },
  });

  const handleDelete = (id: string, name: string) => {
    if (confirm(`Are you sure you want to delete "${name}"? This action cannot be undone.`)) {
      deleteMutation.mutate(id);
    }
  };

  const handleDeploy = (envId: string, envName: string) => {
    const commitSha = prompt(`Enter commit SHA to deploy to ${envName}:`);
    if (commitSha) {
      deployMutation.mutate({ envId, commit_sha: commitSha });
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        {[...Array(5)].map((_, i) => (
          <div key={i} className="animate-pulse">
            <div className="h-24 bg-gray-200 rounded-lg" />
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
        <h3 className="mt-2 text-sm font-medium text-gray-900">Failed to load environments</h3>
        <p className="mt-1 text-sm text-gray-500">{error instanceof Error ? error.message : 'Unknown error'}</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Environments</h1>
          <p className="mt-1 text-sm text-gray-500">
            {project ? `Managing environments for ${project.name}` : 'Select a project to manage environments'}
          </p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          disabled={!projectId}
          className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          New Environment
        </button>
      </div>

      {/* Environments Grid */}
      {environments && environments.length === 0 ? (
        <div className="text-center py-12">
          <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19.429 5l-1.262 12.512a2 2 0 01-1.967 1.988H7.799a2 2 0 01-1.967-1.988L4.571 5M4.571 5h14.858M9.75 5v14m6.75-14v14M4.571 5l.493 4.891a1 1 0 00.979.967h10.358a1 1 0 00.979-.967L19.429 5" />
          </svg>
          <h3 className="mt-2 text-sm font-medium text-gray-900">No environments yet</h3>
          <p className="mt-1 text-sm text-gray-500">Create your first environment to start deploying.</p>
          <button
            onClick={() => setShowCreateModal(true)}
            disabled={!projectId}
            className="mt-4 inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white text-sm font-medium rounded-lg hover:bg-primary-700 disabled:opacity-50"
          >
            <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
            </svg>
            Create Environment
          </button>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {environments?.map((env) => (
            <EnvironmentCard
              key={env.id}
              environment={env}
              onClick={() => setSelectedEnvironment(env)}
              onDelete={() => handleDelete(env.id, env.name)}
              onDeploy={() => handleDeploy(env.id, env.name)}
              isDeploying={deployMutation.isPending}
            />
          ))}
        </div>
      )}

      {/* Create Environment Modal */}
      <CreateEnvironmentModal
        isOpen={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        onSubmit={createMutation.mutate}
        isLoading={createMutation.isPending}
      />
    </div>
  );
}

interface EnvironmentCardProps {
  environment: Environment;
  onClick: () => void;
  onDelete: () => void;
  onDeploy: () => void;
  isDeploying: boolean;
}

function EnvironmentCard({ environment, onClick, onDelete, onDeploy, isDeploying }: EnvironmentCardProps) {
  const getStatusColor = (status?: string) => {
    switch (status) {
      case 'success':
        return 'bg-green-100 text-green-700';
      case 'failed':
        return 'bg-red-100 text-red-700';
      case 'running':
      case 'pending':
        return 'bg-yellow-100 text-yellow-700';
      default:
        return 'bg-gray-100 text-gray-700';
    }
  };

  return (
    <div
      className="group bg-white rounded-xl border border-gray-200 shadow-sm hover:shadow-md transition-all duration-200"
      onClick={onClick}
    >
      <div className="p-5">
        {/* Header */}
        <div className="flex items-start justify-between mb-4">
          <div className="h-10 w-10 rounded-lg bg-blue-100 flex items-center justify-center">
            <span className="text-blue-700 font-medium text-sm">{getInitials(environment.name)}</span>
          </div>
          <div className="opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-1">
            <button
              onClick={(e) => { e.stopPropagation(); onDeploy(); }}
              disabled={isDeploying}
              className="p-1.5 text-gray-400 hover:text-primary-600 rounded-md hover:bg-primary-50 transition-colors disabled:opacity-50"
              aria-label="Deploy to environment"
            >
              <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 17h8m0 0V9m0 8l-8-8-4 4-6-6" />
              </svg>
            </button>
            <button
              onClick={(e) => { e.stopPropagation(); onDelete(); }}
              className="p-1.5 text-gray-400 hover:text-red-600 rounded-md hover:bg-red-50 transition-colors"
              aria-label="Delete environment"
            >
              <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
            </button>
          </div>
        </div>

        {/* Environment info */}
        <h3 className="text-lg font-semibold text-gray-900 truncate mb-1">{environment.name}</h3>
        <div className="flex items-center gap-2 text-sm text-gray-500 mb-3">
          <span className="px-2 py-0.5 bg-gray-100 rounded-full font-mono text-xs">{environment.namespace}</span>
          {environment.cluster && (
            <span className="px-2 py-0.5 bg-blue-50 text-blue-700 rounded-full text-xs">{environment.cluster}</span>
          )}
        </div>

        {/* Last deployment */}
        {environment.last_deployment && (
          <div className="mb-3 p-3 bg-gray-50 rounded-lg">
            <div className="flex items-center justify-between text-sm mb-1">
              <span className="text-gray-500">Last Deployment</span>
              <span className={cn('px-2 py-0.5 rounded-full text-xs font-medium', getStatusColor(environment.last_deployment.status))}>
                {environment.last_deployment.status}
              </span>
            </div>
            <p className="text-xs text-gray-500 font-mono">{environment.last_deployment.id.slice(0, 8)}</p>
            <p className="text-xs text-gray-400">{formatRelativeTime(environment.last_deployment.created_at)}</p>
          </div>
        )}

        {/* Config preview */}
        {environment.config && Object.keys(environment.config).length > 0 && (
          <details className="text-xs text-gray-500">
            <summary className="cursor-pointer">Config: {Object.keys(environment.config).length} keys</summary>
            <pre className="mt-1 p-2 bg-gray-50 rounded text-xs overflow-auto max-h-24">
              {JSON.stringify(environment.config, null, 2)}
            </pre>
          </details>
        )}

        <p className="mt-2 text-xs text-gray-400">Updated {formatRelativeTime(environment.updated_at)}</p>
      </div>
    </div>
  );
}