import { useDispatch, useSelector } from 'react-redux';
import type { AppDispatch, RootState } from './index';

// Give components hooks that know our store's types
export const useAppDispatch = useDispatch.withTypes<AppDispatch>();
export const useAppSelector = useSelector.withTypes<RootState>();