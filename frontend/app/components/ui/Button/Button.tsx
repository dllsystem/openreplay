import React from 'react';
import cn from 'classnames';
import { CircularLoader } from 'UI';

interface Props {
  className?: string;
  children: React.ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  type?: 'button' | 'submit' | 'reset';
  loading?: boolean;
  [x: string]: any
}
export default (props: Props) => {
  const {
    className = '',
    variant = "default",
    type = "button",
    size = '',
    disabled = false,
    children,
    loading = false,
    ...rest
  } = props;

  const classes = ['relative flex items-center h-10 px-3 rounded tracking-wide'];
  if (variant === 'default') {
    classes.push('bg-white hover:bg-gray-lightest border border-gray-light')
  }

  if (variant === 'primary') {
    classes.push('bg-teal color-white hover:bg-teal-dark')
  }

  if (variant === 'text') {
    classes.push('bg-transparent color-gray-dark hover:bg-gray-lightest hover:color-gray-dark')
  }

  if (variant === 'text-primary') {
    classes.push('bg-transparent color-teal hover:bg-teal-light hover:color-teal-dark')
  }

  if (variant === 'outline') {
    classes.push('bg-white color-teal border border-teal hover:bg-teal-light')
  }

  if (disabled) {
    classes.push('opacity-40 pointer-events-none')
  }

  return (
    <button
      { ...rest }
      type={type}
      className={ cn(classes, className ) }
    >
      { loading && <div className="absolute flex items-center justify-center inset-0 z-1 rounded">
        <CircularLoader />
      </div> }
      <div className={cn({ 'opacity-0' : loading }, 'flex items-center')}>{children}</div>
    </button>
  );
}