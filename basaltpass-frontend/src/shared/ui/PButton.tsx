import React, { forwardRef, ButtonHTMLAttributes } from 'react';

interface PButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost' | 'gradient';
  size?: 'sm' | 'md' | 'lg';
  loading?: boolean;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  fullWidth?: boolean;
}

const PButton = forwardRef<HTMLButtonElement, PButtonProps>(
  ({ 
    variant = 'primary',
    size = 'md',
    loading = false,
    leftIcon,
    rightIcon,
    fullWidth = false,
    className = '',
    children,
    disabled,
    ...props 
  }, ref) => {
    const baseStyles = `
      inline-flex items-center justify-center gap-2 border font-medium
      focus:outline-none focus:ring-2 focus:ring-offset-2 
      transition-colors duration-150 ease-in-out
      disabled:cursor-not-allowed disabled:opacity-60
    `;

    const variantStyles = {
      primary: `
        border-transparent text-white bg-indigo-600 
        hover:bg-indigo-700 focus:ring-indigo-500
        shadow-sm
      `,
      secondary: `
        border-gray-300 text-gray-700 bg-white 
        hover:bg-gray-50 focus:ring-indigo-500
        shadow-sm
      `,
      danger: `
        border-transparent text-white bg-red-600 
        hover:bg-red-700 focus:ring-red-500
        shadow-sm
      `,
      ghost: `
        border-transparent text-gray-600 bg-transparent
        hover:bg-gray-100 hover:text-gray-900 focus:ring-indigo-500
      `,
      gradient: `
        border-transparent text-white bg-indigo-600
        hover:bg-indigo-700 focus:ring-indigo-500
        shadow-sm
      `
    };

    const sizeStyles = {
      sm: 'min-h-8 px-3 py-1.5 text-sm rounded-lg',
      md: 'min-h-10 px-4 py-2 text-sm rounded-lg',
      lg: 'min-h-11 px-5 py-2.5 text-base rounded-lg'
    };

    // translatedstyle
    const widthStyles = fullWidth ? 'w-full' : '';

    const buttonClasses = `
      ${baseStyles}
      ${variantStyles[variant]}
      ${sizeStyles[size]}
      ${widthStyles}
      ${className}
    `.replace(/\s+/g, ' ').trim();

    const isDisabled = disabled || loading;

    return (
      <button
        ref={ref}
        className={buttonClasses}
        disabled={isDisabled}
        aria-busy={loading}
        {...props}
      >
        {loading && (
          <svg 
            className={`animate-spin -ml-1 mr-2 h-4 w-4 text-current`} 
            xmlns="http://www.w3.org/2000/svg" 
            fill="none" 
            viewBox="0 0 24 24"
          >
            <circle 
              className="opacity-25" 
              cx="12" 
              cy="12" 
              r="10" 
              stroke="currentColor" 
              strokeWidth="4"
            />
            <path 
              className="opacity-75" 
              fill="currentColor" 
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
        )}
        {!loading && leftIcon && (
          <span className="flex h-4 w-4 shrink-0 items-center justify-center [&>svg]:h-4 [&>svg]:w-4">
            {leftIcon}
          </span>
        )}
        {children}
        {!loading && rightIcon && (
          <span className="flex h-4 w-4 shrink-0 items-center justify-center [&>svg]:h-4 [&>svg]:w-4">
            {rightIcon}
          </span>
        )}
      </button>
    );
  }
);

PButton.displayName = 'PButton';

export default PButton;
