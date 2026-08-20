'use client';

import { useState } from 'react';
import { X } from 'lucide-react';

interface CreateEnvironmentModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (data: { name: string; namespace: string; cluster?: string; config?: Record<string, unknown> }) => void;
  isLoading: boolean;
}

export function CreateEnvironmentModal({ isOpen, onClose, onSubmit, isLoading }: CreateEnvironmentModalProps) {
  const [name, setName] = useState('');
  const [namespace, setNamespace] = useState('');
  const [cluster, setCluster] = useState('');
  const [config, setConfig] = useState('{}');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [configError, setConfigError] = useState<string | null>(null);

  const validate = () => {
    const newErrors: Record<string, string> = {};
    if (!name.trim()) newErrors.name = 'Environment name is required';
    else if (name.length > 50) newErrors.name = 'Name must be 50 characters or less';
    if (!namespace.trim()) newErrors.namespace = 'Kubernetes namespace is required';
    else if (!/^[a-z0-9-]+$/.test(namespace)) newErrors.namespace = 'Namespace must be lowercase alphanumeric with hyphens only';
    else if (namespace.length > 63) newErrors.namespace = 'Namespace must be 63 characters or less';

    // Validate config JSON
    let parsedConfig: Record<string, unknown> = {};
    try {
      parsedConfig = JSON.parse(config);
      if (typeof parsedConfig !== 'object' || parsedConfig === null || Array.isArray(parsedConfig)) {
        throw new Error('Config must be a JSON object');
      }
      setConfigError(null);
    } catch {
      setConfigError('Invalid JSON format');
      newErrors.config = 'Invalid JSON';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0 && !configError;
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!validate()) return;

    let parsedConfig: Record<string, unknown> = {};
    try {
      parsedConfig = JSON.parse(config);
    } catch {
      // Already validated
    }

    onSubmit({
      name: name.trim(),
      namespace: namespace.trim(),
      cluster: cluster.trim() || undefined,
      config: Object.keys(parsedConfig).length > 0 ? parsedConfig : undefined,
    });
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto" role="dialog" aria-modal="true" aria-labelledby="modal-title">
      <div className="flex min-h-full items-center justify-center p-4">
        <div className="fixed inset-0 bg-gray-900/50 transition-opacity" onClick={onClose} aria-hidden="true" />

        <div className="relative w-full max-w-lg bg-white rounded-xl shadow-xl transform transition-all max-h-[90vh] overflow-hidden">
          <div className="flex items-center justify-between p-4 border-b border-gray-200">
            <h2 id="modal-title" className="text-lg font-semibold text-gray-900">Create New Environment</h2>
            <button
              onClick={onClose}
              disabled={isLoading}
              className="p-1 text-gray-400 hover:text-gray-600 rounded-md hover:bg-gray-100 transition-colors disabled:opacity-50"
              aria-label="Close modal"
            >
              <X className="h-5 w-5" />
            </button>
          </div>

          <form onSubmit={handleSubmit} className="p-4 space-y-4 overflow-y-auto max-h-[calc(90vh-140px)]">
            <div>
              <label htmlFor="name" className="block text-sm font-medium text-gray-700 mb-1">
                Environment Name <span className="text-red-500">*</span>
              </label>
              <input
                id="name"
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className={`w-full px-3 py-2 border rounded-lg shadow-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 sm:text-sm ${
                  errors.name ? 'border-red-500' : 'border-gray-300'
                }`}
                placeholder="production"
                disabled={isLoading}
                autoFocus
              />
              {errors.name && <p className="mt-1 text-sm text-red-600">{errors.name}</p>}
            </div>

            <div>
              <label htmlFor="namespace" className="block text-sm font-medium text-gray-700 mb-1">
                Kubernetes Namespace <span className="text-red-500">*</span>
              </label>
              <input
                id="namespace"
                type="text"
                value={namespace}
                onChange={(e) => setNamespace(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
                className={`w-full px-3 py-2 border rounded-lg shadow-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 sm:text-sm ${
                  errors.namespace ? 'border-red-500' : 'border-gray-300'
                }`}
                placeholder="prod"
                disabled={isLoading}
              />
              {errors.namespace && <p className="mt-1 text-sm text-red-600">{errors.namespace}</p>}
              <p className="mt-1 text-xs text-gray-500">Lowercase alphanumeric with hyphens only (K8s namespace format)</p>
            </div>

            <div>
              <label htmlFor="cluster" className="block text-sm font-medium text-gray-700 mb-1">
                Cluster (optional)
              </label>
              <input
                id="cluster"
                type="text"
                value={cluster}
                onChange={(e) => setCluster(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg shadow-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                placeholder="aws-prod-cluster"
                disabled={isLoading}
              />
            </div>

            <div>
              <label htmlFor="config" className="block text-sm font-medium text-gray-700 mb-1">
                Configuration (JSON, optional)
              </label>
              <textarea
                id="config"
                value={config}
                onChange={(e) => setConfig(e.target.value)}
                rows={6}
                className={`w-full px-3 py-2 border rounded-lg shadow-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 sm:text-sm font-mono text-xs ${
                  configError ? 'border-red-500' : 'border-gray-300'
                }`}
                placeholder='{"replicas": 3, "resources": {"limits": {"cpu": "1000m", "memory": "1Gi"}}}'
                disabled={isLoading}
              />
              {configError && <p className="mt-1 text-sm text-red-600">{configError}</p>}
              <p className="mt-1 text-xs text-gray-500">Valid JSON object for environment-specific configuration</p>
            </div>

            <div className="flex gap-3 pt-2">
              <button
                type="button"
                onClick={onClose}
                disabled={isLoading}
                className="flex-1 px-4 py-2 border border-gray-300 text-sm font-medium rounded-lg text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 transition-colors disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={isLoading}
                className="flex-1 px-4 py-2 text-sm font-medium rounded-lg text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isLoading ? 'Creating...' : 'Create Environment'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  );
}