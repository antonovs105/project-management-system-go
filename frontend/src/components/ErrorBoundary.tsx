import React, { Component, type ErrorInfo, type ReactNode } from 'react';

interface Props {
    children?: ReactNode;
}

interface State {
    hasError: boolean;
    error: Error | null;
}

class ErrorBoundary extends Component<Props, State> {
    public state: State = {
        hasError: false,
        error: null,
    };

    public static getDerivedStateFromError(error: Error): State {
        return { hasError: true, error };
    }

    public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
        console.error("Uncaught error:", error, errorInfo);
    }

    public render() {
        if (this.state.hasError) {
            return (
                <div className="p-8 flex flex-col items-center justify-center min-h-screen bg-slate-50 text-slate-800">
                    <h1 className="text-4xl font-bold mb-4">Oops!</h1>
                    <p className="text-xl mb-4">Something went wrong.</p>
                    <div className="bg-red-50 border border-red-200 p-4 rounded-lg max-w-2xl overflow-auto text-red-700 font-mono text-sm">
                        {this.state.error?.message}
                        <br />
                        {this.state.error?.stack}
                    </div>
                    <button
                        className="mt-6 px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
                        onClick={() => window.location.reload()}
                    >
                        Reload Page
                    </button>
                </div>
            );
        }

        return this.props.children;
    }
}

export default ErrorBoundary;
