import {createRoot} from "react-dom/client";
import {Futures} from "./futures";
import "./futures.css";
import "./report.css";

createRoot(document.getElementById("app")!).render(<Futures />);
