import { EventEmitter } from 'events';

export interface Shape {
  area(): number;
  perimeter(): number;
}

export type Point = {
  x: number;
  y: number;
};

export enum Direction {
  North = 'NORTH',
  South = 'SOUTH',
  East = 'EAST',
  West = 'WEST',
}

export namespace Geometry {
  export function distance(a: Point, b: Point): number {
    return Math.sqrt((b.x - a.x) ** 2 + (b.y - a.y) ** 2);
  }
}

export class Circle implements Shape {
  constructor(private radius: number) {}

  area(): number {
    return Math.PI * this.radius ** 2;
  }

  perimeter(): number {
    return 2 * Math.PI * this.radius;
  }

  scale(factor: number): Circle {
    return new Circle(this.radius * factor);
  }
}

export class Rectangle implements Shape {
  constructor(private width: number, private height: number) {}

  area(): number {
    return this.width * this.height;
  }

  perimeter(): number {
    return 2 * (this.width + this.height);
  }
}

export function createCircle(radius: number): Circle {
  return new Circle(radius);
}

export const identity = <T>(x: T): T => x;
