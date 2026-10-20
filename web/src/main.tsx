import React from 'react';
import ReactDOM from 'react-dom/client';
import {HeroUIProvider} from '@heroui/react';
import App from './App';
import './styles.css';
import './readability.css';
import './neobrutalism.css';
import './creator-layout.css';
ReactDOM.createRoot(document.getElementById('root')!).render(<React.StrictMode><HeroUIProvider locale="zh-CN" reducedMotion="user"><App/></HeroUIProvider></React.StrictMode>);
